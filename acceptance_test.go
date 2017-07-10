package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/mattbostock/athensdb/remote"
	promAPI "github.com/prometheus/client_golang/api"
	promAPIv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

var baseURL string

func TestSimpleArithmeticQuery(t *testing.T) {
	query := "1+1"
	expected := "2"

	result, err := queryAPI(query, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	got := result.(*model.Scalar).Value.String()
	if got != expected {
		t.Fatalf("Expected %s, got %s", got, expected)
	}
}

func TestRemoteWrite(t *testing.T) {
	testSample := model.Sample{
		Metric:    make(model.Metric, 1),
		Value:     1234,
		Timestamp: model.Now(),
	}
	testSample.Metric[model.MetricNameLabel] = "foo"

	req := generateRemoteRequest(testSample)
	resp, err := postWriteRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("Expected HTTP status 200, got %d", resp.StatusCode)
	}
}

func TestMain(m *testing.M) {
	flag.Parse()

	// Use localhost to avoid firewall warnings when running tests under OS X.
	config.listenAddr = "localhost:9080"

	baseURL = fmt.Sprintf("http://%s", config.listenAddr)
	go main()

	err := waitForServer(baseURL)
	if err != nil {
		log.Fatal("Test setup failed:", err)
	}

	os.Exit(m.Run())
}

func queryAPI(query string, ts time.Time) (model.Value, error) {
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

func generateRemoteRequest(sample model.Sample) *remote.WriteRequest {
	req := &remote.WriteRequest{
		Timeseries: make([]*remote.TimeSeries, 0, 1),
	}
	ts := &remote.TimeSeries{
		Labels: make([]*remote.LabelPair, 0, len(sample.Metric)),
	}
	for k, v := range sample.Metric {
		ts.Labels = append(ts.Labels,
			&remote.LabelPair{
				Name:  string(k),
				Value: string(v),
			})
	}
	ts.Samples = []*remote.Sample{
		{
			Value:       float64(sample.Value),
			TimestampMs: int64(sample.Timestamp),
		},
	}
	req.Timeseries = append(req.Timeseries, ts)
	return req
}

func postWriteRequest(req *remote.WriteRequest) (*http.Response, error) {
	data, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}

	compressed := snappy.Encode(nil, data)
	u := fmt.Sprintf("%s%s", baseURL, writeRoute)
	resp, err := http.Post(u, "snappy", bytes.NewBuffer(compressed))
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	return resp, nil
}

func waitForServer(u string) error {
	c := make(chan error, 1)
	go func() {
		for {
			_, err := http.Get(u)
			if err == nil {
				c <- nil
				break
			}

			switch err.(type) {
			case *url.Error:
				if strings.HasSuffix(err.Error(), "connection refused") {
					time.Sleep(100 * time.Millisecond)
					continue
				}
			}

			c <- err
		}
	}()

	select {
	case err := <-c:
		return err
	case <-time.After(10 * time.Second):
		return errors.New("timed out wating for server to start")
	}
}
