package testutil

import (
	"bytes"
	"context"
	"math/rand"
	"time"

	promAPI "github.com/prometheus/client_golang/api"
	promAPIv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// FIXME: Test distribution of randomness
// FIXME: Test for uniqueness of time-series generated
// FIXME: Support characters other than lowercase letters
func GenerateDataSamples(numSamples int, seed int64, timeStep time.Duration) []model.Sample {
	r := rand.New(rand.NewSource(seed))
	samples := make([]model.Sample, 0, numSamples)
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
		s := model.Sample{
			Metric:    metric,
			Value:     model.SampleValue(r.Float64()),
			Timestamp: now.Add(timeStep * time.Duration(i)),
		}

		labelName := model.LabelName(model.MetricNameLabel)
		buf.Reset()
		for j := 0; j < numLabels; j++ {
			if j > 0 {
				for k := 0; k < r.Intn(maxLabelNameLength-minLabelNameLength)+minLabelNameLength; k++ {
					buf.WriteRune(rune(int('a') + rand.Intn(lettersInAlphabet)))
				}
				labelName = model.LabelName(buf.String())
			}

			buf.Reset()
			for l := 0; l < r.Intn(maxLabelValueLength-1)+1; l++ {
				buf.WriteRune(rune(int('a') + rand.Intn(lettersInAlphabet)))
			}
			s.Metric[labelName] = model.LabelValue(buf.String())
		}

		samples = append(samples, s)
	}
	return samples
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
