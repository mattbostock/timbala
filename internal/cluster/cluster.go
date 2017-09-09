package cluster

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"sort"
	"strconv"
	"time"

	"github.com/golang/groupcache/consistenthash"
	"github.com/hashicorp/memberlist"
	"github.com/sirupsen/logrus"
)

const (
	hashringVnodes       = 160
	primaryKeyDateFormat = "20060102"
)

var (
	c = struct {
		c    *Config
		ml   *memberlist.Memberlist
		ring *consistenthash.Map
		// Replication factor should never change except in tests.
		// It's not a constant because it is changed during the tests.
		replicationFactor int
	}{
		replicationFactor: 3,
	}
	log *logrus.Logger
)

func SetLogger(l *logrus.Logger) {
	log = l
}

type Config struct {
	HTTPAdvertiseAddr net.TCPAddr
	HTTPBindAddr      net.TCPAddr
	PeerAdvertiseAddr net.TCPAddr
	PeerBindAddr      net.TCPAddr
	Peers             []string
}

func Join(config *Config) error {
	// FIXME(mbostock): Consider using a non-local config for memberlist
	memberConf := memberlist.DefaultLocalConfig()

	memberConf.AdvertiseAddr = config.PeerAdvertiseAddr.IP.String()
	memberConf.AdvertisePort = config.PeerAdvertiseAddr.Port
	memberConf.BindAddr = config.PeerBindAddr.IP.String()
	memberConf.BindPort = config.PeerBindAddr.Port

	memberConf.Delegate = &delegate{}
	memberConf.Events = &eventDelegate{}
	memberConf.LogOutput = ioutil.Discard
	c.c = config
	// FIXME: Make dynamic with cluster size else distribution will degrade
	// as nodes are added
	c.ring = consistenthash.New(c.replicationFactor*hashringVnodes, nil)

	var err error
	if c.ml, err = memberlist.Create(memberConf); err != nil {
		return fmt.Errorf("failed to configure cluster settings: %s", err)
	}
	c.ml.Join(config.Peers)
	return nil
}

type Node struct {
	mln *memberlist.Node
}

func (n *Node) meta() (m nodeMeta, err error) {
	err = json.Unmarshal(n.mln.Meta, &m)
	return
}

func (n *Node) Name() string {
	return n.mln.Name
}
func (n *Node) Addr() string {
	return n.mln.Address()
}
func (n *Node) HTTPAddr() (string, error) {
	m, err := n.meta()
	if err != nil {
		return "", err
	}
	return m.HTTPAddr, nil
}
func (n *Node) String() string {
	return n.Name()
}

func LocalNode() *Node {
	if c.ml == nil {
		panic("Not yet joined a cluster")
	}
	return &Node{c.ml.LocalNode()}
}

func GetNodes() Nodes {
	if c.ml == nil {
		panic("Not yet joined a cluster")
	}
	nodes := make(Nodes, 0, len(c.ml.Members()))
	for _, n := range c.ml.Members() {
		nodes = append(nodes, &Node{n})
	}
	return nodes
}

type Nodes []*Node

func (nodes Nodes) Len() int           { return len(nodes) }
func (nodes Nodes) Less(i, j int) bool { return nodes[i].Name() < nodes[j].Name() }
func (nodes Nodes) Swap(i, j int)      { nodes[i], nodes[j] = nodes[j], nodes[i] }

func (nodes Nodes) FilterBySeries(salt []byte, timestamp time.Time) Nodes {
	// FIXME cache hashmap of names to nodes?
	retNodes := make(Nodes, 0, len(nodes))
	nodesUsed := make(map[*Node]bool, len(nodes))
	useNextNode := false
	pKey := partitionKey(salt, timestamp)

	// Sort nodes to ensure function is deterministic
	sort.Stable(nodes)

	for i := 0; i < c.replicationFactor; i++ {
		nodeName := c.ring.Get(strconv.Itoa(i) + pKey)
		for len(nodesUsed) < c.replicationFactor && len(nodesUsed) < len(nodes) {
			for _, n := range nodes {
				if n.Name() == nodeName || useNextNode {
					if _, ok := nodesUsed[n]; ok {
						useNextNode = true
						continue
					}
					retNodes = append(retNodes, n)
					nodesUsed[n] = true
					useNextNode = false
				}
			}
		}
	}
	return retNodes
}

func partitionKey(salt []byte, end time.Time) string {
	// FIXME filter quantile and le when hashing for data locality?
	buf := make([]byte, 0, len(salt)+len(primaryKeyDateFormat))
	buf = append(buf, salt...)
	buf = append(buf, end.Format(primaryKeyDateFormat)...)
	return string(buf)
}

type delegate struct{}

func (d *delegate) NodeMeta(limit int) []byte {
	// FIXME respect limit
	j, _ := json.Marshal(&nodeMeta{
		HTTPAddr: c.c.HTTPAdvertiseAddr.String(),
	})
	return j
}

func (d *delegate) NotifyMsg([]byte) {}

func (d *delegate) GetBroadcasts(overhead int, limit int) [][]byte {
	return [][]byte{}
}

func (d *delegate) LocalState(join bool) []byte {
	return []byte{}
}

func (d *delegate) MergeRemoteState(buf []byte, join bool) {}

type nodeMeta struct {
	HTTPAddr string `json:"http_addr"`
}

type eventDelegate struct{}

func (e *eventDelegate) NotifyJoin(n *memberlist.Node) {
	log.Infof("Node joined: %s on %s", n.Name, n.Address())
	c.ring.Add(n.Name)
}

func (e *eventDelegate) NotifyLeave(n *memberlist.Node) {
	log.Infof("Node left cluster: %s on %s", n.Name, n.Address())
	// FIXME remove node from ring
}

func (e *eventDelegate) NotifyUpdate(n *memberlist.Node) {
	log.Infof("Node updated: %s on %s", n.Name, n.Address())
}
