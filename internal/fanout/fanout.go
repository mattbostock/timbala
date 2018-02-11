package fanout

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/mattbostock/timbala/internal/cluster"
	"github.com/mattbostock/timbala/internal/read"
	"github.com/prometheus/prometheus/storage"
	"github.com/sirupsen/logrus"
)

const readTimeoutSeconds = 30 * time.Second

var httpClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			DualStack: true,
			KeepAlive: 10 * time.Minute,
			Timeout:   2 * time.Second,
		}).DialContext,
		ExpectContinueTimeout: 5 * time.Second,
		IdleConnTimeout:       10 * time.Minute,
		ResponseHeaderTimeout: 5 * time.Second,
	}}

type fanoutStorage struct {
	clstr      cluster.Cluster
	localStore storage.Storage
	log        *logrus.Logger
}

func New(c cluster.Cluster, l *logrus.Logger, s storage.Storage) *fanoutStorage {
	return &fanoutStorage{
		clstr:      c,
		localStore: s,
		log:        l,
	}
}

func (f *fanoutStorage) Appender() (storage.Appender, error) {
	return newFanoutAppender(f.clstr, f.localStore), nil
}

func (f *fanoutStorage) Querier(ctx context.Context, mint int64, maxt int64) (storage.Querier, error) {
	localQuerier, err := f.localStore.Querier(ctx, mint, maxt)
	if err != nil {
		return nil, err
	}
	queriers := append([]storage.Querier{}, localQuerier)

	// FIXME handle cluster node membership changes
	for _, n := range f.clstr.Nodes() {
		if n.Name() == f.clstr.LocalNode().Name() {
			continue
		}

		httpAddr, err := n.HTTPAddr()
		if err != nil {
			return nil, err
		}

		queriers = append(queriers, remoteQuerier{
			ctx:  ctx,
			maxt: maxt,
			mint: mint,
			// FIXME handle HTTPS
			url: "http://" + httpAddr + read.Route,
		})
	}

	return storage.NewMergeQuerier(queriers), nil
}

func (f *fanoutStorage) StartTime() (int64, error) {
	return 0, nil
}

func (f *fanoutStorage) Close() error {
	return nil
}
