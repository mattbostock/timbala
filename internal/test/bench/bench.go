// +build bench

package main

import (
	"math"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"sort"
	"time"

	"github.com/mattbostock/athensdb/internal/test/testutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/tsdb/labels"
	"github.com/retailnext/hllpp"
	log "github.com/sirupsen/logrus"
)

const (
	applicationName   = "bench"
	samplesToGenerate = 1e5
)

var (
	athensDBAddr = []string{
		"http://athensdb_1:9080",
		"http://athensdb_2:9080",
		"http://athensdb_3:9080",
	}
	uniqueSeries = hllpp.New()

	numUniqueSeries = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: applicationName,
		Name:      "unique_series",
		Help:      "Number of unique series generated (unique combination of name and labels)",
	})
	samplesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: applicationName,
		Name:      "samples_sent_total",
		Help:      "Number of samples sent to AthensDB",
	})
	writeRequestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: applicationName,
		Name:      "write_requests_total",
		Help:      "Number of successful write requests sent to AthensDB",
	})
)

func init() {
	prometheus.MustRegister(numUniqueSeries)
	prometheus.MustRegister(samplesTotal)
	prometheus.MustRegister(writeRequestsTotal)
}

func main() {
	workersPerNode := 4 * int(math.Min(1, float64(runtime.NumCPU()/len(athensDBAddr))))

	for _, url := range athensDBAddr {
		for i := 0; i < workersPerNode; i++ {
			go func(url string) {
				for {
					samples := testutil.GenerateDataSamples(samplesToGenerate, 1, time.Duration(0))
					req := testutil.GenerateRemoteRequest(samples)
					samplesTotal.Add(float64(len(req.Timeseries)))

					resp, err := testutil.PostWriteRequest(url, req)
					if err != nil {
						log.Fatal(err)
					}
					if resp.StatusCode != http.StatusOK {
						log.Fatalf("Expected HTTP status %d, got %d", http.StatusOK, resp.StatusCode)
					}

					for _, ts := range req.Timeseries {
						m := make(labels.Labels, 0, len(ts.Labels))
						for _, l := range ts.Labels {
							m = append(m, labels.Label{
								Name:  l.Name,
								Value: l.Value,
							})
						}
						sort.Stable(m)
						uniqueSeries.Add([]byte(m.String()))
					}

					numUniqueSeries.Set(float64(uniqueSeries.Count()))
					samplesTotal.Add(float64(len(req.Timeseries)))
					writeRequestsTotal.Inc()
				}
			}(url)
		}
	}

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":9000", nil))
}
