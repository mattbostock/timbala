package write

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"sync"

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

	appender, err := store.Appender()
	if err != nil {
		// FIXME: Make error more useful
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer appender.Commit()

	var samples uint32
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
			// FIXME: Look at using AddFast
			appender.Add(m, s.TimestampMs, s.Value)
			samples++
		}
	}

	// This is an internal write, so don't replicate it to other nodes
	if r.Header.Get(HttpHeaderInternalWrite) != "" {
		log.Debugf("Received %d samples from another node in the cluster", samples)
		return
	}

	var wg sync.WaitGroup
	// FIXME: Avoid panic if the cluster is not yet initialised
	nodes := cluster.Nodes()
	var wgErrChan = make(chan error, len(nodes))
	for _, node := range nodes {
		if node.Name() == cluster.LocalNode().Name() {
			log.Debugf("Skipping local node %s", node)
			continue
		}

		wg.Add(1)
		go func(n *cluster.Node) {
			defer wg.Done()

			log.Debugf("Writing %d samples to %s", samples, n)

			httpAddr, err := n.HTTPAddr()
			if err != nil {
				wgErrChan <- err
				return
			}
			apiURL := fmt.Sprintf("%s%s%s", "http://", httpAddr, Route)
			nodeReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(compressed))
			if err != nil {
				wgErrChan <- err
				return
			}
			nodeReq.Header.Add("Content-Encoding", "snappy")
			nodeReq.Header.Set("Content-Type", "application/x-protobuf")
			nodeReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
			nodeReq.Header.Set(HttpHeaderInternalWrite, "0.0.1")

			// FIXME set timeout using context
			httpResp, err := ctxhttp.Do(context.TODO(), http.DefaultClient, nodeReq)
			if err != nil {
				wgErrChan <- err
				return
			}
			defer httpResp.Body.Close()
		}(node)
	}
	// FIXME cancel requests if one fails
	wg.Wait()

	select {
	case err := <-wgErrChan:
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	default:
	}
}
