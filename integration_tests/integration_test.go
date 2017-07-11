// +build integration

package integration

import (
	"testing"
	"time"

	helpers "github.com/mattbostock/athensdb/test_helpers"
	"github.com/prometheus/common/model"
)

const athensDBAddr = "http://athensdb:9080"

func TestPrometheusMetricsCanBeQueried(t *testing.T) {
	// Wait long enough for `scrape_interval` to pass, as specified in `prometheus.yml`
	time.Sleep(2 * time.Second)

	query := `prometheus_build_info`
	result, err := helpers.QueryAPI(athensDBAddr, query, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	expected := model.SampleValue(1)
	got := result.(model.Vector)[0].Value
	if got != expected {
		t.Fatalf("Expected %s, got %s", expected, got)
	}
}
