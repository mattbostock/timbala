package cluster

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/groupcache/consistenthash"
	"github.com/mattbostock/athensdb/internal/test/testutil"
	"github.com/montanaflynn/stats"
	"github.com/prometheus/common/model"
)

const (
	numSamples   = 1e5
	numTestNodes = 11
)

var samples []model.Sample

func TestMain(m *testing.M) {
	samples = testutil.GenerateDataSamples(numSamples, 1, 24*time.Hour)
	os.Exit(m.Run())
}

func TestHashringDistribution(t *testing.T) {
	testSampleDistribution(t, 1, samples)
}

func TestHashringDistributionWithReplication(t *testing.T) {
	testSampleDistribution(t, replicationFactor, samples)
}

func testSampleDistribution(t *testing.T, replFactor int, samples []model.Sample) {
	var buckets [][]model.Sample
	c.ring = consistenthash.New(replicationFactor*hashringVnodes, nil)
	// Add mock nodes to ring
	for i := 0; i < numTestNodes; i++ {
		c.ring.Add(strconv.Itoa(i))
		// Make buckets large enough to hold a node's share plus some extra if distribution is poor
		buckets = append(buckets, make([]model.Sample, 0, (numSamples/(numTestNodes-1))))
	}

	for _, s := range samples {
		for i := 0; i < replFactor; i++ {
			// FIXME: Work out chunk end timestamp
			node, err := strconv.Atoi(c.ring.Get(strconv.Itoa(i) + SeriesPrimaryKey([]byte(""), s.Timestamp.Time())))
			if err != nil {
				t.Fatal(err)
			}
			buckets[node] = append(buckets[node], s)
		}
	}

	fmt.Printf("Distribution of samples when replication factor is %d:\n\n", replFactor)
	var sampleData stats.Float64Data

	for i := 0; i < len(buckets); i++ {
		percent := float64(len(buckets[i])) / float64(len(samples)*replFactor) * 100
		fmt.Printf("Node %-2d: %-100s %5.2f%%; %d samples\n", i, strings.Repeat("#", int(percent)), percent, len(buckets[i]))
		sampleData = append(sampleData, float64(len(buckets[i])))
	}

	min, err := sampleData.Min()
	if err != nil {
		t.Fatal(err)
	}
	max, err := sampleData.Max()
	if err != nil {
		t.Fatal(err)
	}
	mean, err := sampleData.Mean()
	if err != nil {
		t.Fatal(err)
	}
	median, err := sampleData.Median()
	if err != nil {
		t.Fatal(err)
	}
	stddev, err := sampleData.StandardDeviationPopulation()
	if err != nil {
		t.Fatal(err)
	}
	sum, err := sampleData.Sum()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("\nMin: %.0f", min)
	fmt.Printf("\nMax: %.0f", max)
	fmt.Printf("\nMean: %.2f", mean)
	fmt.Printf("\nMedian: %.0f", median)
	fmt.Printf("\nStandard deviation: %.2f", stddev)
	fmt.Printf("\nTotal samples: %.0f\n\n", sum)

	if min == 0 {
		t.Fatal("Some nodes received zero samples")
	}
	if sum != float64(numSamples*replFactor) {
		t.Fatalf("Not all samples accounted for, found %.0f but expected %.0f", sum, numSamples*replFactor)
	}
	if stddev > float64(numSamples/10) {
		t.Fatalf("Samples not well distributed, standard deviation is %.2f for %d samples over %d nodes", stddev, numSamples*replFactor, numTestNodes)
	}
}
