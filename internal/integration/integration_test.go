// +build integration

package integration

import (
	"sync"
	"testing"
	"time"

	"github.com/mattbostock/athensdb/internal/testutil"
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
