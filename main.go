package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/mattbostock/athens/storage"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/remote"
	v1API "github.com/prometheus/prometheus/web/api/v1"
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
	var (
		ctx, cancelCtx = context.WithCancel(context.Background())
		storage        = &storage.RocksDB{}
		queryEngine    = promql.NewEngine(storage, promql.DefaultEngineOptions)
	)
	defer cancelCtx()

	storage.Start()
	defer storage.Stop()

	router := route.New(func(r *http.Request) (context.Context, error) {
		return ctx, nil
	})

	var api = v1API.NewAPI(queryEngine, storage)
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

		for _, ts := range req.Timeseries {
			m := make(model.Metric, len(ts.Labels))
			for _, l := range ts.Labels {
				m[model.LabelName(l.Name)] = model.LabelValue(l.Value)
			}
			fmt.Println(m)

			for _, s := range ts.Samples {
				fmt.Printf("  %f %d\n", s.Value, s.TimestampMs)
				storage.Append(&model.Sample{m, model.SampleValue(s.Value), model.Time(s.TimestampMs)})
			}
		}
	})

	log.Fatal(http.ListenAndServe(config.listenAddr, router))
}
