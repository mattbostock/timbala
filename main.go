package main

import (
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	v1API "github.com/mattbostock/athensdb/api/v1"
	"github.com/mattbostock/athensdb/remote"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/route"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/tsdb"
)

const (
	apiRoute   = "/api/v1"
	writeRoute = "/receive"
)

var config = struct {
	listenAddr string
}{
	":9080",
}

func init() {
	if len(os.Getenv("ADDR")) > 0 {
		config.listenAddr = os.Getenv("ADDR")
	}
}

func main() {
	// FIXME: Review tsdb options
	localStorage, err := tsdb.Open("data", nil, &tsdb.Options{
		AppendableBlocks: 2,
		MinBlockDuration: 2 * time.Hour,
		MaxBlockDuration: 36 * time.Hour,
		Retention:        15 * 24 * time.Hour,
	})
	if err != nil {
		log.Errorf("Opening storage failed: %s", err)
		os.Exit(1)
	}

	var (
		ctx, cancelCtx = context.WithCancel(context.Background())
		queryEngine    = promql.NewEngine(localStorage, promql.DefaultEngineOptions)
	)
	defer cancelCtx()

	router := route.New(func(r *http.Request) (context.Context, error) {
		return ctx, nil
	})

	var api = v1API.NewAPI(queryEngine, localStorage)
	api.Register(router.WithPrefix(apiRoute))

	router.Post("/receive", func(w http.ResponseWriter, r *http.Request) {
		reqBuf, err := ioutil.ReadAll(snappy.NewReader(r.Body))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req remote.WriteRequest
		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		appender, err := localStorage.Appender()
		defer appender.Commit()

		if err != nil {
			// FIXME: Make error more useful
			http.Error(w, err.Error(), http.StatusBadRequest)
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
				// FIXME: Look at using AddFast
				appender.Add(m, s.TimestampMs, s.Value)
			}
		}
	})

	log.Fatal(http.ListenAndServe(config.listenAddr, router))
}
