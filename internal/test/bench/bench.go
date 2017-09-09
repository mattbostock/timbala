// +build bench

package main

import (
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/mattbostock/athensdb/internal/test/testutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
)

const (
	applicationName   = "bench"
	baseURL           = "http://load_balancer"
	samplesToGenerate = 1e5
)

var (
	samplesQueued = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: applicationName,
		Name:      "samples_queued",
		Help:      "Samples queues to be sent to AthensDB",
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
	prometheus.MustRegister(samplesQueued)
	prometheus.MustRegister(samplesTotal)
	prometheus.MustRegister(writeRequestsTotal)
}

func main() {
	sampleChan := make(chan model.Samples, 3)
	go func() {
		for {
			sampleChan <- testutil.GenerateDataSamples(samplesToGenerate, 1, time.Duration(0))
			samplesQueued.Set(float64(len(sampleChan)) * samplesToGenerate)
		}
	}()
	// FIXME: Make number of nodes in the cluster configurable for
	// benchmarking large clusters
	for i := 0; i < 3; i++ {
		go func() {
			for samples := range sampleChan {
				req := testutil.GenerateRemoteRequest(samples)
				resp, err := testutil.PostWriteRequest(baseURL, req)
				if err != nil {
					log.Fatal(err)
				}
				if resp.StatusCode != 200 {
					log.Fatalf("Expected HTTP status 200, got %d", resp.StatusCode)
				}
				samplesTotal.Add(float64(len(samples)))
				writeRequestsTotal.Inc()
			}
		}()
	}

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":9000", nil))
}
