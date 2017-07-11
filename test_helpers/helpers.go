package test_helpers

import (
	"context"
	"time"

	promAPI "github.com/prometheus/client_golang/api"
	promAPIv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

func QueryAPI(baseURL, query string, ts time.Time) (model.Value, error) {
	conf := promAPI.Config{
		Address: baseURL,
	}
	client, err := promAPI.NewClient(conf)
	if err != nil {
		return nil, err
	}

	api := promAPIv1.NewAPI(client)
	values, err := api.Query(context.TODO(), query, ts)
	if err != nil {
		return nil, err
	}

	return values, err
}
