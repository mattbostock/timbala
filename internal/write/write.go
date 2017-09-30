package write

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/golang/snappy"
	"github.com/mattbostock/timbala/internal/cluster"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context/ctxhttp"
)

const (
	HTTPHeaderInternalWrite        = "X-Timbala-Internal-Write-Version"
	HTTPHeaderInternalWriteVersion = "0.0.1"
	HTTPHeaderPartitionKeySalt     = "X-Timbala-Partition-Key-Salt"
	Route                          = "/receive"

	httpHeaderRemoteWrite        = "X-Prometheus-Remote-Write-Version"
	httpHeaderRemoteWriteVersion = "0.1.0"
	numPreallocTimeseries        = 1e5
)

type Writer interface {
	Handler(http.ResponseWriter, *http.Request)
}

type writer struct {
	clstr cluster.Cluster
	store storage.Storage
	log   *logrus.Logger
	mu    sync.Mutex
}

func New(c cluster.Cluster, l *logrus.Logger, s storage.Storage) *writer {
	return &writer{
		clstr: c,
		log:   l,
		store: s,
	}
}

func (wr *writer) Handler(w http.ResponseWriter, r *http.Request) {
	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		wr.log.Warningln(err)
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

	if len(req.Timeseries) == 0 {
		err := errors.New("received empty request containing zero timeseries")
		wr.log.Warningln(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// This is an internal write, so don't replicate it to other nodes
	// This case is very common, to make it fast
	if r.Header.Get(HTTPHeaderInternalWrite) != "" {
		wr.mu.Lock()
		appender, err := wr.store.Appender()
		if err != nil {
			wr.mu.Unlock()
			wr.log.Warning(err)
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
		wr.mu.Unlock()

		wr.log.Debugf("Wrote %d series received from another node in the cluster", len(req.Timeseries))
		return
	}

	// FIXME handle change in cluster size
	seriesToNodes := make(seriesNodeMap, len(wr.clstr.Nodes()))
	for _, n := range wr.clstr.Nodes() {
		seriesToNodes[*n] = make(seriesMap, numPreallocTimeseries)
	}

	pSalt := []byte(r.Header.Get(HTTPHeaderPartitionKeySalt))
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
			pKey := cluster.PartitionKey(pSalt, timestamp, mHash)
			for _, n := range wr.clstr.NodesByPartitionKey(pKey) {
				if _, ok := seriesToNodes[*n][mHash]; !ok {
					// FIXME handle change in cluster size
					seriesToNodes[*n][mHash] = &prompb.TimeSeries{
						Labels:  ts.Labels,
						Samples: make([]*prompb.Sample, 0, len(ts.Samples)),
					}
				}
				seriesToNodes[*n][mHash].Samples = append(seriesToNodes[*n][mHash].Samples, s)
			}
		}
		// FIXME: sort samples by time?
	}

	localSeries, ok := seriesToNodes[*wr.clstr.LocalNode()]
	if ok {
		err = wr.localWrite(localSeries)
		if err != nil {
			wr.log.Warningln(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Remove local node so that it's not written to again as a 'remote' node
		delete(seriesToNodes, *wr.clstr.LocalNode())
	}

	err = wr.remoteWrite(seriesToNodes)
	if err != nil {
		wr.log.Warningln(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (wr *writer) localWrite(series seriesMap) error {
	wr.mu.Lock()
	appender, err := wr.store.Appender()
	if err != nil {
		wr.mu.Unlock()
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
	wr.mu.Unlock()
	return nil
}

func (wr *writer) remoteWrite(sNodeMap seriesNodeMap) error {
	var wg sync.WaitGroup
	var wgErrChan = make(chan error, len(wr.clstr.Nodes()))
	for node, nodeSeries := range sNodeMap {
		if len(nodeSeries) == 0 {
			continue
		}

		wg.Add(1)
		go func(n cluster.Node, nSeries seriesMap) {
			defer wg.Done()

			wr.log.Debugf("Writing %d series to %s", len(nSeries), n.Name())

			httpAddr, err := n.HTTPAddr()
			if err != nil {
				wgErrChan <- err
				return
			}
			apiURL := fmt.Sprintf("%s%s%s", "http://", httpAddr, Route)

			req := &prompb.WriteRequest{
				Timeseries: make([]*prompb.TimeSeries, 0, len(nSeries)),
			}
			for _, ts := range nSeries {
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
			nodeReq.Header.Set(httpHeaderRemoteWrite, httpHeaderRemoteWriteVersion)
			nodeReq.Header.Set(HTTPHeaderInternalWrite, HTTPHeaderInternalWriteVersion)

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
		}(node, nodeSeries)
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

type seriesNodeMap map[cluster.Node]seriesMap
type seriesMap map[uint64]*prompb.TimeSeries
