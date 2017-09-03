package cluster

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/groupcache/consistenthash"
	"github.com/hashicorp/memberlist"
	"github.com/mattbostock/athensdb/internal/test/testutil"
	"github.com/montanaflynn/stats"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
)

const (
	numSamples int = 1e5
)

var (
	samples          []model.Sample
	testClusterSizes = [...]int{1, replicationFactor, 5, 11, 59}
)

func TestMain(m *testing.M) {
	samples = testutil.GenerateDataSamples(numSamples, 1, 24*time.Hour)
	os.Exit(m.Run())
}

func TestHashringDistribution(t *testing.T) {
	for _, numTestNodes := range testClusterSizes {
		testSampleDistribution(t, numTestNodes, samples)
	}
}

func testSampleDistribution(t *testing.T, numTestNodes int, samples []model.Sample) {
	var (
		buckets   [][]model.Sample
		mockNodes Nodes
	)

	c.ring = consistenthash.New(replicationFactor*hashringVnodes, nil)

	// Add mock nodes to ring
	for i := 0; i < numTestNodes; i++ {
		c.ring.Add(strconv.Itoa(i))
		buckets = append(buckets, make([]model.Sample, 0, numSamples))
		mockNodes = append(mockNodes, &Node{mln: &memberlist.Node{Name: strconv.Itoa(i)}})
	}

	var replicationSpread stats.Float64Data
	for _, s := range samples {
		spread := make(map[int]bool)
		for _, n := range mockNodes.FilterBySeries([]byte{}, labels.Labels{}, s.Timestamp.Time()) {
			i, err := strconv.Atoi(n.Name())
			if err != nil {
				t.Fatal(err)
			}

			buckets[i] = append(buckets[i], s)
			spread[i] = true
		}
		replicationSpread = append(replicationSpread, float64(len(spread)))
	}

	fmt.Printf("Distribution of samples when replication factor is %d across a cluster of %d nodes:\n\n", replicationFactor, numTestNodes)
	var sampleData stats.Float64Data

	for i := 0; i < len(buckets); i++ {
		percent := float64(len(buckets[i])) / float64(len(samples)*replicationFactor) * 100
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
	fmt.Print("\nSummary:")
	fmt.Printf("\nMin: %.0f", min)
	fmt.Printf("\nMax: %.0f", max)
	fmt.Printf("\nMean: %.2f", mean)
	fmt.Printf("\nMedian: %.0f", median)
	fmt.Printf("\nStandard deviation: %.2f", stddev)
	fmt.Printf("\nTotal samples: %.0f\n\n", sum)

	replMin, err := replicationSpread.Min()
	if err != nil {
		t.Fatal(err)
	}
	replMax, err := replicationSpread.Max()
	if err != nil {
		t.Fatal(err)
	}
	replMode, err := replicationSpread.Mode()
	if err != nil {
		t.Fatal(err)
	}
	replMean, err := replicationSpread.Mean()
	if err != nil {
		t.Fatal(err)
	}

	fmt.Print("Distribution of replicas across nodes:\n\n")
	for i := 0; i <= int(replMax); i++ {
		samplesInBucket := 0
		for _, j := range replicationSpread {
			if i == int(j) {
				samplesInBucket++
			}
		}

		percent := float64(samplesInBucket) / float64(len(samples)) * 100
		fmt.Printf("%-2d nodes: %-100s %5.2f%%; %d samples\n", i, strings.Repeat("#", int(percent)), percent, samplesInBucket)

		if i == 0 && samplesInBucket > 0 {
			t.Fatalf("%d samples were not allocated to any nodes", samplesInBucket)
		}
	}

	fmt.Print("\nReplication summary:")
	fmt.Printf("\nMin nodes samples are spread over: %.0f", replMin)
	fmt.Printf("\nMax nodes samples are spread over: %.0f", replMax)
	fmt.Printf("\nMode nodes samples are spread over: %.0f", replMode)
	fmt.Printf("\nMean nodes samples are spread over: %.2f\n", replMean)

	if len(replicationSpread) != len(samples) {
		t.Fatalf("Not all samples accounted for in replication spread summary; expected %d, got %d", len(replicationSpread), len(samples))
	}

	if replMean != replicationFactor && replMean < float64(numTestNodes) {
		t.Fatalf("Samples are not replicated across exactly %d nodes", replicationFactor)
	}

	if min == 0 {
		t.Fatal("Some nodes received zero samples")
	}
	if expected := float64(numSamples) * math.Min(float64(replicationFactor), float64(numTestNodes)); sum != expected {
		t.Fatalf("Not all samples accounted for, found %.0f but expected %.0f", sum, expected)
	}
	if stddev > float64(numSamples/10) {
		t.Fatalf("Samples not well distributed, standard deviation is %.2f for %d samples over %d nodes", stddev, numSamples*replicationFactor, numTestNodes)
	}
}
