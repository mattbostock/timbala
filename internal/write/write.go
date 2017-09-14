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

	"golang.org/x/net/context/ctxhttp"

	"github.com/golang/snappy"
	"github.com/mattbostock/athensdb/internal/cluster"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/sirupsen/logrus"
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

	if len(req.Timeseries) == 0 {
		err := errors.New("received empty request containing zero timeseries")
		log.Warningln(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	series := make([]*seriesData, 0, numPreallocTimeseries)
	for _, ts := range req.Timeseries {
		m := make(labels.Labels, 0, len(ts.Labels))
		for _, l := range ts.Labels {
			m = append(m, labels.Label{
				Name:  l.Name,
				Value: l.Value,
			})
		}
		sort.Stable(m)

		series = append(series, &seriesData{
			labels:         m,
			proto:          *ts,
			samplesToNodes: make([][]string, 0, len(ts.Samples)),
		})
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

		for _, ts := range series {
			for _, s := range ts.proto.Samples {
				// FIXME: Look at using AddFast
				appender.Add(ts.labels, s.Timestamp, s.Value)
			}
		}
		appender.Commit()
		mu.Unlock()

		log.Debugf("Wrote %d series received from another node in the cluster", len(series))
		return
	}

	mu.Lock()
	// FIXME look at using multiple appenders
	appender, err := store.Appender()
	if err != nil {
		mu.Unlock()
		log.Warningln(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	for _, ts := range series {
		for i, s := range ts.proto.Samples {
			timestamp := time.Unix(s.Timestamp/1000, (s.Timestamp-s.Timestamp/1000)*1e6)
			// FIXME: Avoid panic if the cluster is not yet initialised
			for _, n := range cluster.GetNodes().FilterBySeries([]byte{}, timestamp) {
				// Required so that the length of ts.samplesToNodes matches number of samples
				if len(ts.samplesToNodes) == i {
					ts.samplesToNodes = append(ts.samplesToNodes, make([]string, 0, cluster.ReplicationFactor))
				}

				if n.Name() == cluster.LocalNode().Name() {
					// FIXME: Look at using AddFast
					appender.Add(ts.labels, s.Timestamp, s.Value)
					continue
				}

				ts.samplesToNodes[i] = append(ts.samplesToNodes[i], n.Name())
			}
		}
	}
	var wg sync.WaitGroup
	// FIXME handle change in cluster size
	var wgErrChan = make(chan error, len(cluster.GetNodes()))

	for _, node := range cluster.GetNodes() {
		// FIXME check not a local node, panic if so

		wg.Add(1)
		go func(n *cluster.Node) {
			defer wg.Done()

			httpAddr, err := n.HTTPAddr()
			if err != nil {
				wgErrChan <- err
				return
			}
			apiURL := fmt.Sprintf("%s%s%s", "http://", httpAddr, Route)

			req := &prompb.WriteRequest{
				// FIXME reduce this?
				Timeseries: make([]*prompb.TimeSeries, 0, len(series)),
			}

			for _, ts := range series {
				added := false
			sampleLoop:
				for i, sn := range ts.samplesToNodes {
					// optimise this by freezing the slice of nodes and using the index?
					for _, sNode := range sn {
						if sNode == n.Name() {
							if !added {
								req.Timeseries = append(req.Timeseries, &prompb.TimeSeries{
									Labels: ts.proto.Labels,
									// FIXME preallocate samples?
								})
								added = true
							}
							j := len(req.Timeseries) - 1
							req.Timeseries[j].Samples = append(req.Timeseries[j].Samples, ts.proto.Samples[i])
							break sampleLoop
						}
					}
				}
			}

			//	log.Debugf("Writing %d series to %s", len(nSeries), n.Name())

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
				wgErrChan <- fmt.Errorf("got HTTP %d status code from %s", httpResp.StatusCode, n.Name())
				return
			}
		}(node)
	}
	// FIXME cancel requests if one fails
	wg.Wait()

	select {
	case err := <-wgErrChan:
		return err
	default:
	}
	// Intentionally avoid defer on hot path
	appender.Commit()
	mu.Unlock()
}

type seriesData struct {
	proto          prompb.TimeSeries
	labels         labels.Labels
	samplesToNodes [][]string
}
