package acceptance_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"syscall"
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

// FIXME: Ensure that the binary is the one output by the Makefile and not some
// other binary found in $PATH
const executable = "timbala"

// FIXME: Set this explicitly when executing the binary
var httpBaseURL = "http://localhost:9080"

func TestPprof(t *testing.T) {
	c := run()
	defer teardown(c)

	resp, err := http.Get(httpBaseURL + "/debug/pprof/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(body), "profiles:") {
		t.Fatal("pprof endpoint inaccessible")
	}
}

func TestMetrics(t *testing.T) {
	c := run()
	defer teardown(c)

	resp, err := http.Get(httpBaseURL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(body), "timbala_build_info") {
		t.Fatal("No build_info metric found")
	}
	if !strings.Contains(string(body), "prometheus_engine") {
		t.Fatal("No Prometheus query engine metrics found")
	}
	if !strings.Contains(string(body), "http_request") {
		t.Fatal("No HTTP API metrics found")
	}
	if !strings.Contains(string(body), "go_info") {
		t.Fatal("No Go runtime metrics found")
	}
	if !strings.Contains(string(body), "tsdb_head_samples_appended_total") {
		t.Fatal("No tsdb metrics found")
	}
}

func TestSimpleArithmeticQuery(t *testing.T) {
	c := run()
	defer teardown(c)

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
	c := run()
	defer teardown(c)

	testSample := &model.Sample{
		Metric:    make(model.Metric, 1),
		Value:     1234,
		Timestamp: model.Now(),
	}
	testSample.Metric[model.MetricNameLabel] = model.LabelValue(t.Name())

	req := testutil.GenerateRemoteRequest(model.Samples{testSample})
	resp, err := testutil.PostWriteRequest(httpBaseURL, req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected HTTP status %d, got %d", http.StatusOK, resp.StatusCode)
	}
}

func TestRemoteWriteThenQueryBack(t *testing.T) {
	c := run()
	defer teardown(c)

	metricName := t.Name()
	testSample := &model.Sample{
		Metric:    make(model.Metric, 1),
		Value:     1234,
		Timestamp: model.Now(),
	}
	testSample.Metric[model.MetricNameLabel] = model.LabelValue(metricName)

	req := testutil.GenerateRemoteRequest(model.Samples{testSample})
	resp, err := testutil.PostWriteRequest(httpBaseURL, req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected HTTP status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	result, err := testutil.QueryAPI(httpBaseURL, metricName, time.Now())
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

func TestRemoteWriteThenRemoteReadBack(t *testing.T) {
	c := run()
	defer teardown(c)

	now := model.Now()
	metricName := t.Name()
	testSample := &model.Sample{
		Metric:    make(model.Metric, 1),
		Value:     1234,
		Timestamp: now,
	}
	testSample.Metric[model.MetricNameLabel] = model.LabelValue(metricName)

	req := testutil.GenerateRemoteRequest(model.Samples{testSample})
	resp, err := testutil.PostWriteRequest(httpBaseURL, req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected HTTP status %d, got %d", http.StatusOK, resp.StatusCode)
	}

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
	httpReq, err := http.NewRequest("POST", httpBaseURL+read.Route, bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("Unable to create request: %v", err)
	}
	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("X-Prometheus-Remote-Read-Version", "0.1.0")

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

	if len(readResp.Results) == 0 {
		t.Fatal("Got no results")
		return
	}

	if len(readResp.Results[0].Timeseries) == 0 {
		t.Fatal("Got no timeseries in result")
		return
	}

	expected := &prompb.TimeSeries{
		Labels: []*prompb.Label{
			&prompb.Label{
				Name:  labels.MetricName,
				Value: metricName,
			},
		},
		Samples: []*prompb.Sample{
			&prompb.Sample{
				Timestamp: now.UnixNano() / int64(time.Millisecond),
				Value:     1234.0,
			},
		},
	}
	got := readResp.Results[0].Timeseries[0]
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Expected %v, got %v", expected, got)
	}
}

func run(args ...string) *exec.Cmd {
	cmd := exec.Command(executable, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err := cmd.Start()
	if err != nil {
		panic(err)
	}
	waitForServer(httpBaseURL)
	return cmd
}

func waitForServer(u string) {
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
		if err != nil {
			panic(err)
		}
	case <-time.After(10 * time.Second):
		panic("timed out wating for server to start")
	}
}

func teardown(cmd *exec.Cmd) {
	_ = cmd.Process.Signal(syscall.SIGTERM)
	_ = cmd.Wait()
	// FIXME use more specific directory name
	_ = os.RemoveAll("./data")
}
