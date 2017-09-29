// +build !race

package cluster

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cespare/xxhash"
	"github.com/hashicorp/memberlist"
	"github.com/mattbostock/athensdb/internal/hashring"
	"github.com/mattbostock/athensdb/internal/test/testutil"
	"github.com/montanaflynn/stats"
	"github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"
)

const (
	histogramDisplayWidth     = 50
	numSamples            int = 1e5
)

var (
	samples                model.Samples
	testClusterSizes       = [...]int{1, DefaultReplFactor, 19}
	testReplicationFactors = [...]int{1, DefaultReplFactor, 19}
)

func TestMain(m *testing.M) {
	samples = testutil.GenerateDataSamples(numSamples, 1, time.Second)
	os.Exit(m.Run())
}

func BenchmarkHashringDistribution(b *testing.B) {
	ml := newMockMemberlist(DefaultReplFactor, 19)
	clstr := &cluster{
		ml:         ml,
		log:        logrus.StandardLogger(),
		replFactor: DefaultReplFactor,
		ring:       hashring.New(),
	}

	now := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clstr.NodesByPartitionKey(uint64(now.Add(time.Duration(i)).Unix()))
	}
}

func TestHashringDistribution(t *testing.T) {
	for _, numTestNodes := range testClusterSizes {
		for _, replFactor := range testReplicationFactors {
			ml := newMockMemberlist(replFactor, numTestNodes)
			clstr := &cluster{
				ml:         ml,
				log:        logrus.StandardLogger(),
				replFactor: replFactor,
				ring:       hashring.New(),
			}

			t.Run(fmt.Sprintf("%d replicas across %d nodes", replFactor, numTestNodes),
				func(t *testing.T) {
					testSampleDistribution(t, clstr, samples)
				})
		}
	}
}

func testSampleDistribution(t *testing.T, clstr Cluster, samples model.Samples) {
	var buckets = make(map[string]model.Samples, len(clstr.Nodes()))

	var replicationSpread stats.Float64Data
	for _, s := range samples {
		spread := make(map[string]bool)
		pKey := PartitionKey([]byte{}, s.Timestamp.Time(), xxhash.Sum64String(s.Metric.String()))
		for _, n := range clstr.NodesByPartitionKey(pKey) {
			buckets[n.Name()] = append(buckets[n.Name()], s)
			spread[n.Name()] = true
		}
		replicationSpread = append(replicationSpread, float64(len(spread)))
	}

	fmt.Printf("Distribution of samples when replication factor is %d across a cluster of %d nodes:\n\n", clstr.ReplicationFactor(), len(clstr.Nodes()))
	var sampleData stats.Float64Data

	for k := range buckets {
		percent := float64(len(buckets[k])) / float64(len(samples)*clstr.ReplicationFactor()) * 100
		format := "Node %-2s: %-" + strconv.Itoa(histogramDisplayWidth) + "s %5.2f%%; %d samples\n"
		fmt.Printf(format, k, strings.Repeat("#", int(percent/(100/histogramDisplayWidth))), percent, len(buckets[k]))
		sampleData = append(sampleData, float64(len(buckets[k])))
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
	fmt.Printf("\nMean: %.0f", mean)
	fmt.Printf("\nMedian: %.0f", median)
	fmt.Printf("\nStandard deviation: %.0f (%.0f ±%.2f%% RSD)", stddev, mean, stddev*100/mean)
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

	fmt.Printf("Distribution of %d replicas across %d nodes:\n\n", clstr.ReplicationFactor(), len(clstr.Nodes()))
	for i := 0; i <= int(replMax); i++ {
		samplesInBucket := 0
		for _, j := range replicationSpread {
			if i == int(j) {
				samplesInBucket++
			}
		}

		percent := float64(samplesInBucket) / float64(len(samples)) * 100
		format := "%-2d nodes: %-" + strconv.Itoa(histogramDisplayWidth) + "s %5.2f%%; %d samples\n"
		fmt.Printf(format, i, strings.Repeat("#", int(percent/(100/histogramDisplayWidth))), percent, samplesInBucket)

		if i == 0 && samplesInBucket > 0 {
			t.Fatalf("%d samples were not allocated to any nodes", samplesInBucket)
		}
	}

	fmt.Print("\nReplication summary:")
	fmt.Printf("\nMin nodes samples are spread over: %.0f", replMin)
	fmt.Printf("\nMax nodes samples are spread over: %.0f", replMax)
	fmt.Printf("\nMode nodes samples are spread over: %.0f", replMode)
	fmt.Printf("\nMean nodes samples are spread over: %.2f\n\n", replMean)

	if len(replicationSpread) != len(samples) {
		t.Fatalf("Not all samples accounted for in replication spread summary; expected %d, got %d", len(replicationSpread), len(samples))
	}

	if replMean != float64(clstr.ReplicationFactor()) && replMean < float64(len(clstr.Nodes())) {
		t.Fatalf("Samples are not replicated across exactly %d nodes", clstr.ReplicationFactor())
	}

	if min == 0 {
		t.Fatal("Some nodes received zero samples")
	}
	if expected := float64(numSamples) * math.Min(float64(clstr.ReplicationFactor()), float64(len(clstr.Nodes()))); sum != expected {
		t.Fatalf("Not all samples accounted for, found %.0f but expected %.0f", sum, expected)
	}
	if stddev > float64(numSamples/10) {
		t.Fatalf("Samples not well distributed, standard deviation is %.2f for %d samples over %d nodes", stddev, numSamples*clstr.ReplicationFactor(), len(clstr.Nodes()))
	}
}

type mockMemberlist struct {
	nodes Nodes
}

func (m *mockMemberlist) Nodes() Nodes {
	return m.nodes

}

func (m *mockMemberlist) LocalNode() *Node {
	// There's no special significant for the first node, any node will do
	return m.nodes[0]
}

func newMockMemberlist(replFactor, numNodes int) *mockMemberlist {
	nodes := make(Nodes, 0, numNodes)
	for i := 0; i < numNodes; i++ {
		nodes = append(nodes, &Node{&memberlist.Node{Name: strconv.Itoa(i)}})
	}

	return &mockMemberlist{nodes}
}
