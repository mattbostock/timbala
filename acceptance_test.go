package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/mattbostock/athensdb/internal/remote"
	"github.com/mattbostock/athensdb/internal/testutil"
	"github.com/prometheus/common/model"
)

var httpBaseURL string

func TestSimpleArithmeticQuery(t *testing.T) {
	query := "1+1"
	expected := "2"

	result, err := testutil.QueryAPI(httpBaseURL, query, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	got := result.(*model.Scalar).Value.String()
	if got != expected {
		t.Fatalf("Expected %s, got %s", expected, got)
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

func TestRemoteWriteThenQueryBack(t *testing.T) {
	name := time.Now().Format("test_2006_01_02T15_04_05")

	testSample := model.Sample{
		Metric:    make(model.Metric, 1),
		Value:     1234,
		Timestamp: model.Now(),
	}
	testSample.Metric[model.MetricNameLabel] = model.LabelValue(name)

	req := generateRemoteRequest(testSample)
	resp, err := postWriteRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("Expected HTTP status 200, got %d", resp.StatusCode)
	}

	result, err := testutil.QueryAPI(httpBaseURL, name, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	expected := model.SampleValue(1234.0)

	if len(result.(model.Vector)) == 0 {
		t.Fatalf("Got 0 results")
	}

	got := result.(model.Vector)[0].Value
	if got != expected {
		t.Fatalf("Expected %s, got %s", expected, got)
	}
}

func TestMain(m *testing.M) {
	// Use localhost to avoid firewall warnings when running tests under OS X.
	config.httpBindAddr, _ = net.ResolveTCPAddr("tcp", defaultHTTPAddr)
	config.peerBindAddr, _ = net.ResolveTCPAddr("tcp", defaultPeerAddr)

	httpBaseURL = fmt.Sprintf("http://%s", config.httpBindAddr)
	go main()

	err := waitForServer(httpBaseURL)
	if err != nil {
		log.Fatal("Test setup failed: ", err)
	}

	os.Exit(m.Run())
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
	u := fmt.Sprintf("%s%s", httpBaseURL, writeRoute)
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
