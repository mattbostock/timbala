package testutil

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/golang/snappy"
	promAPI "github.com/prometheus/client_golang/api"
	promAPIv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

// FIXME: Test distribution of randomness
// FIXME: Test for uniqueness of time-series generated
// FIXME: Support characters other than lowercase letters
func GenerateDataSamples(numSamples int, seed int64, timeStep time.Duration) model.Samples {
	r := rand.New(rand.NewSource(seed))
	samples := make(model.Samples, 0, numSamples)
	now := model.Now()

	maxNumLabels := 7
	minLabelNameLength := 4
	maxLabelNameLength := 16
	maxLabelValueLength := 18
	lettersInAlphabet := 26

	var buf bytes.Buffer
	for i := 0; i < numSamples; i++ {
		numLabels := r.Intn(maxNumLabels-1) + 1
		metric := make(model.Metric, numLabels)
		s := &model.Sample{
			Metric:    metric,
			Value:     model.SampleValue(r.Float64()),
			Timestamp: now.Add(timeStep * time.Duration(i)),
		}

		labelName := model.LabelName(model.MetricNameLabel)
		buf.Reset()
		for j := 0; j < numLabels; j++ {
			if j > 0 {
				for k := 0; k < r.Intn(maxLabelNameLength-minLabelNameLength)+minLabelNameLength; k++ {
					buf.WriteRune(rune(int('a') + r.Intn(lettersInAlphabet)))
				}
				labelName = model.LabelName(buf.String())
			}

			buf.Reset()
			for l := 0; l < r.Intn(maxLabelValueLength-1)+1; l++ {
				buf.WriteRune(rune(int('a') + r.Intn(lettersInAlphabet)))
			}
			s.Metric[labelName] = model.LabelValue(buf.String())
		}

		samples = append(samples, s)
	}
	return samples
}

func GenerateRemoteRequest(samples model.Samples) *prompb.WriteRequest {
	req := &prompb.WriteRequest{
		Timeseries: make([]*prompb.TimeSeries, 0, len(samples)),
	}
	for _, s := range samples {
		ts := &prompb.TimeSeries{
			Labels: make([]*prompb.Label, 0, len(s.Metric)),
		}
		for k, v := range s.Metric {
			ts.Labels = append(ts.Labels,
				&prompb.Label{
					Name:  string(k),
					Value: string(v),
				})
		}
		ts.Samples = []*prompb.Sample{
			{
				Value:     float64(s.Value),
				Timestamp: int64(s.Timestamp),
			},
		}
		req.Timeseries = append(req.Timeseries, ts)
	}
	return req
}

func QueryAPI(baseURL, query string, ts time.Time) (model.Value, error) {
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

func PostWriteRequest(baseURL string, req *prompb.WriteRequest) (*http.Response, error) {
	data, err := req.Marshal()
	if err != nil {
		return nil, err
	}

	compressed := snappy.Encode(nil, data)
	u := fmt.Sprintf("%s%s", baseURL, "/receive")
	resp, err := http.Post(u, "snappy", bytes.NewBuffer(compressed))
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	return resp, nil
}
