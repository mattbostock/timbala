package main

import (
	"net/http"

	"golang.org/x/net/context"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/route"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
	v1API "github.com/prometheus/prometheus/web/api/v1"
)

func main() {
	var (
		ctx, cancelCtx = context.WithCancel(context.Background())
		storage        = &local.NoopStorage{}
		queryEngine    = promql.NewEngine(storage, promql.DefaultEngineOptions)
	)
	defer cancelCtx()

	router := route.New(func(r *http.Request) (context.Context, error) {
		return ctx, nil
	})

	var api = v1API.NewAPI(queryEngine, storage)
	api.Register(router.WithPrefix("/api/v1"))
	log.Fatal(http.ListenAndServe(":9080", router))
}
