package hashring

import "github.com/golang/groupcache/consistenthash"

func New(replicationFactor, vnodes int) *hashRing {
	return &hashRing{consistenthash.New(replicationFactor*vnodes, nil)}
}

type hashRing struct {
	*consistenthash.Map
}

type HashRing interface {
	Add(...string)
	Get(string) string
}
