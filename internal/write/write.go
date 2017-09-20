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

	var wg sync.WaitGroup
	var series []*seriesData
	// FIXME handle change in cluster size
	var wgErrChan = make(chan error, len(cluster.GetNodes()))

	// FIXME look at using multiple appenders
	appender, err := store.Appender()
	if err != nil {
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

		if r.Header.Get(HttpHeaderInternalWrite) != "" {
			for _, sa := range ts.Samples {
				// FIXME: Look at using AddFast
				mu.Lock()
				appender.Add(m, sa.Timestamp, sa.Value)
				mu.Unlock()
			}
			appender.Commit()
			return
		}
		series = append(series, &seriesData{proto: ts, labels: m})
	}

	for _, node := range cluster.GetNodes() {
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

			for _, s := range series {
				for i, sa := range s.proto.Samples {
					found := false
					timestamp := time.Unix(sa.Timestamp/1000, (sa.Timestamp-sa.Timestamp/1000)*1e6)
					// FIXME: Avoid panic if the cluster is not yet initialised
					for _, n2 := range cluster.GetNodes().FilterBySeries([]byte{}, timestamp) {
						if n2.Name() == cluster.LocalNode().Name() {
							// FIXME AddFast
							mu.Lock()
							appender.Add(s.labels, sa.Timestamp, sa.Value)
							mu.Unlock()
							break
						}

						if n2.Name() == n.Name() {
							found = true
							break
						}
					}

					if !found {
						var end int
						if i == len(s.proto.Samples)-1 {
							end = len(s.proto.Samples) - 1
						} else {
							end = i + 1
						}
						// FIXME memory leak? https://github.com/golang/go/wiki/SliceTricks
						s.proto.Samples = append(s.proto.Samples[:i], s.proto.Samples[end:]...)
					}
				}

				if len(s.proto.Samples) > 0 {
					req.Timeseries = append(req.Timeseries, s.proto)
				}
			}

			log.Debugf("Writing %d series to %s", len(req.Timeseries), n.Name())
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
		log.Warningln(err)
		appender.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		appender.Commit()
	}
}

type seriesData struct {
	proto  *prompb.TimeSeries
	labels labels.Labels
}
