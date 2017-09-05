// +build bench

package main

import (
	"fmt"
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
	applicationName = "bench"
	baseURL         = "http://load_balancer"
)

var (
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
	prometheus.MustRegister(samplesTotal)
	prometheus.MustRegister(writeRequestsTotal)
}

func main() {
	sampleChan := make(chan model.Samples)
	go func() {
		for {
			sampleChan <- testutil.GenerateDataSamples(1e5, 1, 0*time.Second)
		}
	}()
	go func() {
		for samples := range sampleChan {
			err := store(samples)
			if err != nil {
				log.Fatal(err)
			}
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":9000", nil))
}

func store(samples model.Samples) error {
	req := testutil.GenerateRemoteRequest(samples)
	resp, err := testutil.PostWriteRequest(baseURL, req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("Expected HTTP status 200, got %d", resp.StatusCode)
	}
	samplesTotal.Add(float64(len(samples)))
	writeRequestsTotal.Inc()

	return nil
}
