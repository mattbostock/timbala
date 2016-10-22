package main

import (
	"net/http"
	"os"

	"golang.org/x/net/context"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/route"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
	v1API "github.com/prometheus/prometheus/web/api/v1"
)

const apiRoute = "/api/v1"

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
		storage        = &local.NoopStorage{}
		queryEngine    = promql.NewEngine(storage, promql.DefaultEngineOptions)
	)
	defer cancelCtx()

	router := route.New(func(r *http.Request) (context.Context, error) {
		return ctx, nil
	})

	var api = v1API.NewAPI(queryEngine, storage)
	api.Register(router.WithPrefix(apiRoute))

	log.Fatal(http.ListenAndServe(config.listenAddr, router))
}
