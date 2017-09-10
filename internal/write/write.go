package write

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/golang/snappy"
	"github.com/mattbostock/athensdb/internal/cluster"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context/ctxhttp"
)

const (
	HttpHeaderInternalWrite = "X-AthensDB-Internal-Write-Version"
	Route                   = "/receive"

	numPreallocTimeseries = 1e5
)

var (
	store storage.Storage
	log   *logrus.Logger
	mu    sync.Mutex
)

func SetLogger(l *logrus.Logger) {
	log = l
}
func SetStore(s storage.Storage) {
	store = s
}

func Handler(w http.ResponseWriter, r *http.Request) {
	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Warningln(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req = prompb.WriteRequest{Timeseries: make([]*prompb.TimeSeries, 0, numPreallocTimeseries)}
	if err := req.Unmarshal(reqBuf); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// This is an internal write, so don't replicate it to other nodes
	// This case is very common, to make it fast
	if r.Header.Get(HttpHeaderInternalWrite) != "" {
		mu.Lock()
		appender, err := store.Appender()
		if err != nil {
			mu.Unlock()
			log.Warning(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		for _, ts := range req.Timeseries {
			m := make(labels.Labels, 0, len(ts.Labels))
			for _, l := range ts.Labels {
				m = append(m, labels.Label{
					Name:  l.Name,
					Value: l.Value,
				})
			}
			sort.Stable(m)

			for _, s := range ts.Samples {
				// FIXME: Look at using AddFast
				appender.Add(m, s.Timestamp, s.Value)
			}
		}
		appender.Commit()
		mu.Unlock()

		log.Debugf("Wrote %d series received from another node in the cluster", len(req.Timeseries))
		return
	}

	// FIXME handle change in cluster size
	samplesToNodes := make(sampleNodeMap, len(cluster.GetNodes()))
	for _, n := range cluster.GetNodes() {
		samplesToNodes[*n] = make(seriesMap, numPreallocTimeseries)
	}

	for _, ts := range req.Timeseries {
		m := make(labels.Labels, 0, len(ts.Labels))
		for _, l := range ts.Labels {
			m = append(m, labels.Label{
				Name:  l.Name,
				Value: l.Value,
			})
		}
		sort.Stable(m)
		// FIXME: Handle collisions
		mHash := m.Hash()

		for _, s := range ts.Samples {
			timestamp := time.Unix(s.Timestamp/1000, (s.Timestamp-s.Timestamp/1000)*1e6)
			// FIXME: Avoid panic if the cluster is not yet initialised
			for _, n := range cluster.GetNodes().FilterBySeries([]byte{}, timestamp) {
				if _, ok := samplesToNodes[*n][mHash]; !ok {
					// FIXME handle change in cluster size
					samplesToNodes[*n][mHash] = &prompb.TimeSeries{
						Labels:  ts.Labels,
						Samples: make([]*prompb.Sample, 0, len(ts.Samples)),
					}
				}
				samplesToNodes[*n][mHash].Samples = append(samplesToNodes[*n][mHash].Samples, s)
			}
		}
		// FIXME: sort samples by time?
	}

	localSeries, ok := samplesToNodes[*cluster.LocalNode()]
	if ok {
		err = localWrite(localSeries)
		if err != nil {
			log.Warningln(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Remove local node so that it's not written to again as a 'remote' node
		delete(samplesToNodes, *cluster.LocalNode())
	}

	err = remoteWrite(samplesToNodes)
	if err != nil {
		log.Warningln(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func localWrite(series seriesMap) error {
	mu.Lock()
	appender, err := store.Appender()
	if err != nil {
		mu.Unlock()
		return err
	}

	for _, sseries := range series {
		m := make(labels.Labels, 0, len(sseries.Labels))
		for _, l := range sseries.Labels {
			m = append(m, labels.Label{
				Name:  l.Name,
				Value: l.Value,
			})
		}
		sort.Stable(m)

		for _, s := range sseries.Samples {
			// FIXME: Look at using AddFast
			appender.Add(m, s.Timestamp, s.Value)
		}
	}
	// Intentionally avoid defer on hot path
	appender.Commit()
	mu.Unlock()
	return nil
}

func remoteWrite(sampleMap sampleNodeMap) error {
	var wg sync.WaitGroup
	var wgErrChan = make(chan error, len(cluster.GetNodes()))
	for node, nodeSamples := range sampleMap {
		if len(nodeSamples) == 0 {
			continue
		}

		wg.Add(1)
		go func(n cluster.Node, nSamples seriesMap) {
			defer wg.Done()

			log.Debugf("Writing %d samples to %s", len(nSamples), n.Name())

			httpAddr, err := n.HTTPAddr()
			if err != nil {
				wgErrChan <- err
				return
			}
			apiURL := fmt.Sprintf("%s%s%s", "http://", httpAddr, Route)

			req := &prompb.WriteRequest{
				Timeseries: make([]*prompb.TimeSeries, 0, len(nSamples)),
			}
			for _, ts := range nSamples {
				req.Timeseries = append(req.Timeseries, ts)
			}

			data, err := req.Marshal()
			if err != nil {
				wgErrChan <- err
				return
			}

			compressed := snappy.Encode(nil, data)
			nodeReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(compressed))
			if err != nil {
				wgErrChan <- err
				return
			}
			nodeReq.Header.Add("Content-Encoding", "snappy")
			nodeReq.Header.Set("Content-Type", "application/x-protobuf")
			// FIXME move version numbers into constants
			nodeReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
			nodeReq.Header.Set(HttpHeaderInternalWrite, "0.0.1")

			// FIXME set timeout using context
			httpResp, err := ctxhttp.Do(context.TODO(), http.DefaultClient, nodeReq)
			if err != nil {
				wgErrChan <- err
				return
			}

			io.Copy(ioutil.Discard, httpResp.Body)
			httpResp.Body.Close()

			if httpResp.StatusCode != http.StatusOK {
				wgErrChan <- fmt.Errorf("got HTTP %d status code", httpResp.StatusCode)
				return
			}
		}(node, nodeSamples)
	}
	// FIXME cancel requests if one fails
	wg.Wait()

	select {
	case err := <-wgErrChan:
		return err
	default:
	}

	return nil
}

type sampleNodeMap map[cluster.Node]seriesMap
type seriesMap map[uint64]*prompb.TimeSeries
