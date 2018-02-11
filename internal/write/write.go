package write

import (
	"errors"
	"io/ioutil"
	"net/http"
	"sort"
	"sync"

	"github.com/golang/snappy"
	"github.com/mattbostock/timbala/internal/cluster"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/sirupsen/logrus"
)

const (
	HTTPHeaderInternalWrite        = "X-Timbala-Internal-Write-Version"
	HTTPHeaderInternalWriteVersion = "0.0.1"
	HTTPHeaderRemoteWrite          = "X-Prometheus-Remote-Write-Version"
	HTTPHeaderRemoteWriteVersion   = "0.1.0"
	Route                          = "/write"

	numPreallocTimeseries = 1e5
)

type Writer interface {
	HandlerFunc(http.ResponseWriter, *http.Request)
}

type writer struct {
	clstr       cluster.Cluster
	fanoutStore storage.Storage
	localStore  storage.Storage
	log         *logrus.Logger
	mu          sync.Mutex
}

func New(c cluster.Cluster, l *logrus.Logger, s storage.Storage, fo storage.Storage) *writer {
	return &writer{
		clstr:       c,
		fanoutStore: fo,
		log:         l,
		localStore:  s,
	}
}

func (wr *writer) HandlerFunc(w http.ResponseWriter, r *http.Request) {
	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		wr.log.Warningln(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		wr.log.Debug(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req = prompb.WriteRequest{Timeseries: make([]*prompb.TimeSeries, 0, numPreallocTimeseries)}
	if err := req.Unmarshal(reqBuf); err != nil {
		wr.log.Debug(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.Timeseries) == 0 {
		err := errors.New("received empty request containing zero timeseries")
		wr.log.Debug(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var appender storage.Appender
	internal := r.Header.Get(HTTPHeaderInternalWrite) != ""
	if internal {
		// This is an internal write, so don't replicate it to other nodes.
		appender, err = wr.localStore.Appender()
	} else {
		appender, err = wr.fanoutStore.Appender()
	}
	if err != nil {
		wr.log.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, sseries := range req.Timeseries {
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
			// FIXME handle errors
			_, err = appender.Add(m, s.Timestamp, s.Value)
			if err != nil {
				wr.log.Error(err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	// Intentionally avoid defer on hot path
	appender.Commit()
}
