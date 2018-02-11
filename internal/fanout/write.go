package fanout

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"golang.org/x/net/context/ctxhttp"

	"github.com/golang/snappy"
	"github.com/mattbostock/timbala/internal/cluster"
	"github.com/mattbostock/timbala/internal/write"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
)

func newFanoutAppender(c cluster.Cluster, l storage.Storage) *fanoutAppender {
	// FIXME handle change in cluster size
	return &fanoutAppender{
		cluster:       c,
		localStore:    l,
		seriesToNodes: make(seriesNodeMap, len(c.Nodes())),
	}
}

type fanoutAppender struct {
	sync.Mutex

	cluster       cluster.Cluster
	localStore    storage.Storage
	seriesToNodes seriesNodeMap
}

func (a *fanoutAppender) Add(l labels.Labels, t int64, v float64) (uint64, error) {
	// FIXME handle collisions
	ref := l.Hash()
	err := a.AddFast(l, ref, t, v)
	if err != nil {
		return 0, nil
	}

	return ref, nil
}

func (a *fanoutAppender) AddFast(l labels.Labels, ref uint64, t int64, v float64) error {
	// FIXME needed?
	a.Lock()
	defer a.Unlock()

	// FIXME cache this
	pbLabels := make([]*prompb.Label, 0, len(l))
	for _, lb := range l {
		pbLabels = append(pbLabels, &prompb.Label{
			Name:  lb.Name,
			Value: lb.Value,
		})
	}

	timestamp := time.Unix(t/int64(time.Millisecond), (t-t/int64(time.Millisecond))*int64(time.Nanosecond))
	pKey := cluster.PartitionKey(timestamp, ref)
	for _, n := range a.cluster.NodesByPartitionKey(pKey) {
		if _, ok := a.seriesToNodes[n]; !ok {
			a.seriesToNodes[n] = make(seriesMap, 1e5)
		}
		if _, ok := a.seriesToNodes[n][ref]; !ok {
			a.seriesToNodes[n][ref] = &prompb.TimeSeries{
				Labels:  pbLabels,
				Samples: make([]*prompb.Sample, 0, 1),
			}
		}
		a.seriesToNodes[n][ref].Samples = append(a.seriesToNodes[n][ref].Samples, &prompb.Sample{
			Timestamp: t,
			Value:     v,
		})
	}

	return nil
}

func (a *fanoutAppender) Commit() error {
	a.Lock()
	defer a.Unlock()

	for n, series := range a.seriesToNodes {
		ss := seriesMapToSeriesSlice(series)
		if n.Name() == a.cluster.LocalNode().Name() {
			appender, err := a.localStore.Appender()
			if err != nil {
				return err
			}

			for _, ts := range ss {
				// FIXME cache this
				m := make(labels.Labels, 0, len(ts.Labels))
				for _, l := range ts.Labels {
					m = append(m, labels.Label{
						Name:  l.Name,
						Value: l.Value,
					})
				}
				sort.Stable(m)

				for _, sa := range ts.Samples {
					// FIXME handle errors
					// FIXME AddFast?
					_, _ = appender.Add(m, sa.Timestamp, sa.Value)
				}
			}
			// FIXME handle errors
			_ = appender.Commit()
			continue
		}

		httpAddr, err := n.HTTPAddr()
		if err != nil {
			return err
		}

		// FIXME handle HTTPS
		url := "http://" + httpAddr + write.Route

		// FIXME parallelise
		err = remoteWrite(url, context.TODO(), ss)
		if err != nil {
			return err
		}
	}

	// FIXME better way to reset?
	a.seriesToNodes = make(seriesNodeMap, len(a.cluster.Nodes()))

	return nil
}

func (a *fanoutAppender) Rollback() error {
	a.Lock()
	defer a.Unlock()

	// FIXME better way to reset?
	a.seriesToNodes = make(seriesNodeMap, len(a.cluster.Nodes()))
	return nil
}

func seriesMapToSeriesSlice(ss seriesMap) []*prompb.TimeSeries {
	series := make([]*prompb.TimeSeries, 0, len(ss))
	for _, s := range ss {
		series = append(series, s)
	}

	return series
}

func remoteWrite(url string, ctx context.Context, series []*prompb.TimeSeries) error {
	req := &prompb.WriteRequest{
		Timeseries: series,
	}

	data, err := req.Marshal()
	if err != nil {
		return err
	}

	compressed := snappy.Encode(nil, data)
	nodeReq, err := http.NewRequest("POST", url, bytes.NewBuffer(compressed))
	if err != nil {
		return err
	}
	nodeReq.Header.Add("Content-Encoding", "snappy")
	nodeReq.Header.Set("Content-Type", "application/x-protobuf")
	nodeReq.Header.Set(write.HTTPHeaderRemoteWrite, write.HTTPHeaderRemoteWriteVersion)
	nodeReq.Header.Set(write.HTTPHeaderInternalWrite, write.HTTPHeaderInternalWriteVersion)

	httpResp, err := ctxhttp.Do(ctx, http.DefaultClient, nodeReq)
	if err != nil {
		return err
	}
	httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("got HTTP %d status code", httpResp.StatusCode)
	}

	return nil
}

type seriesNodeMap map[*cluster.Node]seriesMap
type seriesMap map[uint64]*prompb.TimeSeries
