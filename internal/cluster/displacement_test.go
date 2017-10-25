// +build !race

package cluster

import (
	"fmt"
	"testing"

	"github.com/cespare/xxhash"
	"github.com/mattbostock/timbala/internal/hashring"
	"github.com/sirupsen/logrus"
)

func TestHashringDisplacement(t *testing.T) {
	clstr := &cluster{
		log:        logrus.StandardLogger(),
		replFactor: DefaultReplFactor,
		ring:       hashring.New(),
	}

	var (
		startClusterSize        = 19
		sampleToNode            = make(map[string]int)
		uniqueReplicatedSamples int
	)

	// Start with n nodes, expand cluster by one, back to n, then reduce by one
	for i, numTestNodes := range []int{startClusterSize, startClusterSize + 1, startClusterSize, startClusterSize - 1} {
		clstr.ml = newMockMemberlist(DefaultReplFactor, numTestNodes)

		for _, s := range samples {
			pKey := PartitionKey([]byte{}, s.Timestamp.Time(), xxhash.Sum64String(s.Metric.String()))
			for _, n := range clstr.NodesByPartitionKey(pKey) {
				sampleToNode[s.Metric.String()+n.Name()]++
			}
		}

		// Only run once
		if i == 0 {
			uniqueReplicatedSamples = len(sampleToNode)
			continue
		}

		// Check results for every pair of cluster sizes
		// FIXME use t.Run() to document what is being tested
		if i%2 == 1 {
			// Accounts for forwarding
			expectedChange := 1.0 / (float64(numTestNodes) / (1.0 + (1.0 / float64(DefaultReplFactor)))) * float64(uniqueReplicatedSamples)
			changed := float64(len(sampleToNode) - uniqueReplicatedSamples)
			if changed > expectedChange {
				t.Fatalf("Expected less than %.0f data samples to change nodes, %.0f changed", expectedChange, changed)
			}

			fmt.Printf("%d unique samples\n", uniqueReplicatedSamples)
			fmt.Printf("At most %.0f samples should change node\n", expectedChange)
			fmt.Printf("%d samples changed node\n", len(sampleToNode)-uniqueReplicatedSamples)

			sampleToNode = make(map[string]int)
		}
	}
}
