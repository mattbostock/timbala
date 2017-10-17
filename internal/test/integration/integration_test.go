// +build integration

package integration

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/mattbostock/timbala/internal/read"
	"github.com/mattbostock/timbala/internal/test/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
)

var timbalaAddr = []string{
	"http://timbala_1:9080",
	"http://timbala_2:9080",
	"http://timbala_3:9080",
}

func TestPrometheusMetricsCanBeQueried(t *testing.T) {
	// Wait long enough for evaluation_interval and scrape_interval to
	// pass, as specified in prometheus.yml, plus an additional 2 seconds
	// to allow time for the data to be ingested to all nodes.
	time.Sleep(4 * time.Second)

	var wg sync.WaitGroup
	for _, a := range timbalaAddr {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()

			query := `prometheus_build_info`
			result, err := testutil.QueryAPI(addr, query, time.Now())
			if err != nil {
				t.Error(err)
				return
			}

			expected := model.SampleValue(1)

			if len(result.(model.Vector)) == 0 {
				t.Error("Got 0 results")
				return
			}

			got := result.(model.Vector)[0].Value
			if got != expected {
				t.Error("Expected %s, got %s", expected, got)
				return
			}
		}(a)
	}

	wg.Wait()
}

func TestQueryAPIFanout(t *testing.T) {
	t.Skip("FIXME: implement query API fanout tests")
}

// Test that the remote read API includes results from all matching nodes in
// the cluster (not just the local node).
func TestRemoteReadFanout(t *testing.T) {
	now := model.Now()
	metricName := t.Name()

	// Send internal writes to 3 nodes, knowing that only those nodes should store each write
	var wg sync.WaitGroup
	for _, a := range timbalaAddr {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()

			testSample := &model.Sample{
				Metric:    make(model.Metric, 1),
				Value:     1234,
				Timestamp: now,
			}
			testSample.Metric[model.MetricNameLabel] = model.LabelValue(metricName)
			testSample.Metric["node"] = model.LabelValue(addr)

			req := testutil.GenerateRemoteRequest(model.Samples{testSample})
			// FIXME add internal header
			resp, err := testutil.PostWriteRequest(addr, req)
			if err != nil {
				t.Fatal(err)
			}

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Expected HTTP status %d, got %d", http.StatusOK, resp.StatusCode)
			}
		}(a)
	}

	wg.Wait()

	// Send a query that matches all 3 writes and check we got back all the data
	queryNode := timbalaAddr[0]

	// FIXME deduplicate this code copied from the acceptance tests
	readReq := &prompb.ReadRequest{
		Queries: []*prompb.Query{{
			StartTimestampMs: now.UnixNano() / int64(time.Millisecond),
			EndTimestampMs:   now.UnixNano() / int64(time.Millisecond),
			Matchers: []*prompb.LabelMatcher{
				&prompb.LabelMatcher{
					Type:  prompb.LabelMatcher_EQ,
					Name:  labels.MetricName,
					Value: metricName,
				},
			},
		}},
	}

	data, err := proto.Marshal(readReq)
	if err != nil {
		t.Fatalf("Unable to marshal read request: %v", err)
	}

	compressed := snappy.Encode(nil, data)
	httpReq, err := http.NewRequest("POST", queryNode+read.Route, bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("Unable to create request: %v", err)
	}
	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("Error sending request: %v", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode/100 != 2 {
		t.Fatalf("Server returned HTTP status %s", httpResp.Status)
	}

	compressed, err = ioutil.ReadAll(httpResp.Body)
	if err != nil {
		t.Fatalf("Error reading response: %v", err)
	}

	uncompressed, err := snappy.Decode(nil, compressed)
	if err != nil {
		t.Fatalf("Error reading response: %v", err)
	}

	var readResp prompb.ReadResponse
	err = proto.Unmarshal(uncompressed, &readResp)
	if err != nil {
		t.Fatalf("Unnable to unmarshal response body: %v", err)
	}

	if len(readResp.Results[0].Timeseries) != 3 {
		t.Fatalf("Got %d timeseries, expected %d", len(readResp.Results[0].Timeseries), 3)
		return
	}
}
