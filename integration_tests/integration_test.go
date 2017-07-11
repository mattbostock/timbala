// +build integration

package integration

import (
	"sync"
	"testing"
	"time"

	helpers "github.com/mattbostock/athensdb/test_helpers"
	"github.com/prometheus/common/model"
)

var athensDBAddr = []string{
	"http://athensdb_1:9080",
	"http://athensdb_2:9080",
	"http://athensdb_3:9080",
}

func TestPrometheusMetricsCanBeQueried(t *testing.T) {
	// Wait long enough for `scrape_interval` to pass, as specified in `prometheus.yml`
	time.Sleep(2 * time.Second)

	var wg sync.WaitGroup
	for _, a := range athensDBAddr {
		wg.Add(1)
		go func(addr string) {
			query := `prometheus_build_info`
			result, err := helpers.QueryAPI(addr, query, time.Now())
			if err != nil {
				t.Fatal(err)
			}

			expected := model.SampleValue(1)
			got := result.(model.Vector)[0].Value
			if got != expected {
				t.Fatalf("Expected %s, got %s", expected, got)
			}
			wg.Done()
		}(a)
	}

	wg.Wait()
}
