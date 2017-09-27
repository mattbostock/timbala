package hashring

import jump "github.com/dgryski/go-jump"

func New() *hashRing {
	return &hashRing{}
}

type hashRing struct{}

func (h *hashRing) Get(key uint64, numBuckets int) int32 {
	return jump.Hash(key, numBuckets)
}

type HashRing interface {
	Get(uint64, int) int32
}
