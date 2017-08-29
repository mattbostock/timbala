package write

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/mattbostock/athensdb/internal/cluster"
	"github.com/mattbostock/athensdb/internal/remote"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"golang.org/x/net/context/ctxhttp"
)

const (
	HttpHeaderInternalWrite = "X-AthensDB-Internal-Write-Version"
	Route                   = "/receive"
)

var (
	store storage.Storage
	log   *logrus.Logger
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req remote.WriteRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// FIXME handle change in cluster size
	samplesToNodes := make(sampleNodeMap, len(cluster.Nodes()))
	for _, n := range cluster.Nodes() {
		samplesToNodes[*n] = make(seriesMap)
	}

	for _, ts := range req.Timeseries {
		m := make(labels.Labels, 0, len(ts.Labels))
		for _, l := range ts.Labels {
			m = append(m, labels.Label{
				Name:  l.Name,
				Value: l.Value,
			})
		}
		sort.Sort(m)

		for _, s := range ts.Samples {
			timestamp := time.Unix(s.TimestampMs/1000, (s.TimestampMs-s.TimestampMs/1000)*1e6)
			// FIXME: Avoid panic if the cluster is not yet initialised
			nodes := cluster.GetNodesForSeries([]byte{}, m, timestamp)
			for _, n := range nodes {
				if _, ok := samplesToNodes[*n][m.Hash()]; !ok {
					samplesToNodes[*n][m.Hash()] = &timeseries{labels: m}
				}
				samplesToNodes[*n][m.Hash()].samples = append(samplesToNodes[*n][m.Hash()].samples, s)
			}
		}
		// FIXME: sort samples by time?
	}

	localSeries, ok := samplesToNodes[*cluster.LocalNode()]
	if ok {
		err = localWrite(localSeries)
		if err != nil {
			// FIXME make error more useful
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// This is an internal write, so don't replicate it to other nodes
	if r.Header.Get(HttpHeaderInternalWrite) != "" {
		// FIXME raise error if this is supposed to be an internal write but there were none?
		log.Debugf("Received %d series from another node in the cluster", len(localSeries))
		return
	}

	err = remoteWrite(samplesToNodes)
	if err != nil {
		// FIXME make error more useful
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func localWrite(series seriesMap) error {
	appender, err := store.Appender()
	if err != nil {
		return err
	}

	for _, sseries := range series {
		for _, s := range sseries.samples {
			// FIXME: Look at using AddFast
			appender.Add(sseries.labels, s.TimestampMs, s.Value)
		}
	}
	// Intentionally avoid defer on hot path
	appender.Commit()
	return nil
}

func remoteWrite(sampleMap sampleNodeMap) error {
	var wg sync.WaitGroup
	var wgErrChan = make(chan error, len(cluster.Nodes()))
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

			req := &remote.WriteRequest{
				Timeseries: make([]*remote.TimeSeries, 0, len(nSamples)),
			}
			for _, series := range nSamples {
				for _, s := range series.samples {
					ts := &remote.TimeSeries{
						Labels: make([]*remote.LabelPair, 0, len(series.labels)),
					}
					for _, l := range series.labels {
						ts.Labels = append(ts.Labels,
							&remote.LabelPair{
								Name:  l.Name,
								Value: l.Value,
							})
					}
					ts.Samples = []*remote.Sample{
						{
							Value:       float64(s.Value),
							TimestampMs: int64(s.TimestampMs),
						},
					}
					req.Timeseries = append(req.Timeseries, ts)
				}
			}

			data, err := proto.Marshal(req)
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
			defer httpResp.Body.Close()
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

type seriesMap map[uint64]*timeseries

type timeseries struct {
	labels  labels.Labels
	samples []*remote.Sample
}
