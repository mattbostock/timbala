package cluster

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"sort"
	"time"

	"github.com/cespare/xxhash"
	"github.com/hashicorp/memberlist"
	"github.com/mattbostock/athensdb/internal/hashring"
	"github.com/sirupsen/logrus"
)

const (
	primaryKeyDateFormat = "20060102"

	DefaultReplFactor = 3
)

func New(conf *Config, l *logrus.Logger) (*cluster, error) {
	if conf.ReplicationFactor == 0 {
		conf.ReplicationFactor = DefaultReplFactor
	}

	cluster := &cluster{
		log:        l,
		replFactor: conf.ReplicationFactor,
		ring:       hashring.New(),
	}

	// FIXME(mbostock): Consider using a non-local config for memberlist
	memberConf := memberlist.DefaultLocalConfig()
	memberConf.AdvertiseAddr = conf.GossipAdvertiseAddr.IP.String()
	memberConf.AdvertisePort = conf.GossipAdvertiseAddr.Port
	memberConf.BindAddr = conf.GossipBindAddr.IP.String()
	memberConf.BindPort = conf.GossipBindAddr.Port
	memberConf.Delegate = &delegate{
		localHTTPAdvertiseAddr: conf.HTTPAdvertiseAddr.String(),
	}
	memberConf.Events = &eventDelegate{
		cluster: cluster,
		log:     l,
	}
	memberConf.LogOutput = ioutil.Discard

	ml, err := memberlist.Create(memberConf)
	if err != nil {
		return nil, fmt.Errorf("failed to configure cluster settings: %s", err)
	}
	ml.Join(conf.Peers)

	cluster.ml = &membership{ml}
	return cluster, nil
}

func (c *cluster) LocalNode() *Node {
	return c.ml.LocalNode()
}

func (c *cluster) Nodes() Nodes {
	return c.ml.Nodes()
}

func (c *cluster) NodesByPartitionKey(pKey uint64) Nodes {
	nodes := c.Nodes()
	nodesUsed := make(map[*Node]bool, len(nodes))
	retNodes := make(Nodes, 0, len(nodes))

	// Sort nodes to ensure function is deterministic
	sort.Stable(nodes)

	for i := 0; i < c.ReplicationFactor(); i++ {
		if len(nodesUsed) == c.ReplicationFactor() || len(nodesUsed) == len(nodes) {
			break
		}

		hashedNodeIndex := c.HashRing().Get(uint64(i)+pKey, len(nodes))
		useNextNode := false
	nodeLoop:
		for j := 0; ; j++ {
			if j == 2 {
				panic("iterated through all nodes twice and still couldn't find a match")
			}
			for k, n := range nodes {
				if int32(k) == hashedNodeIndex || useNextNode {
					if _, ok := nodesUsed[n]; ok {
						useNextNode = true
						continue
					}
					retNodes = append(retNodes, n)
					nodesUsed[n] = true
					break nodeLoop
				}
			}
		}
	}
	return retNodes
}

func PartitionKey(salt []byte, end time.Time, metricHash uint64) uint64 {
	// FIXME filter quantile and le when hashing for data locality?
	buf := make([]byte, 0, len(salt)+len(primaryKeyDateFormat))
	buf = append(buf, salt...)
	buf = append(buf, end.Format(primaryKeyDateFormat)...)
	return xxhash.Sum64(buf) + metricHash
}

func (c *cluster) ReplicationFactor() int {
	return c.replFactor
}

func (c *cluster) HashRing() hashring.HashRing {
	return c.ring
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

type Nodes []*Node

func (nodes Nodes) Len() int           { return len(nodes) }
func (nodes Nodes) Less(i, j int) bool { return nodes[i].Name() < nodes[j].Name() }
func (nodes Nodes) Swap(i, j int)      { nodes[i], nodes[j] = nodes[j], nodes[i] }

type delegate struct {
	localHTTPAdvertiseAddr string
}

func (d *delegate) NodeMeta(limit int) []byte {
	// FIXME respect limit
	j, _ := json.Marshal(&nodeMeta{
		HTTPAddr: d.localHTTPAdvertiseAddr,
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

type eventDelegate struct {
	cluster *cluster
	log     *logrus.Logger
}

func (e *eventDelegate) NotifyJoin(n *memberlist.Node) {
	e.log.Infof("Node joined: %s on %s", n.Name, n.Address())
}

func (e *eventDelegate) NotifyLeave(n *memberlist.Node) {
	e.log.Infof("Node left cluster: %s on %s", n.Name, n.Address())
	// FIXME remove node from ring
}

func (e *eventDelegate) NotifyUpdate(n *memberlist.Node) {
	e.log.Infof("Node updated: %s on %s", n.Name, n.Address())
}

type cluster struct {
	log        *logrus.Logger
	ml         Membership
	replFactor int
	ring       hashring.HashRing
}

type Config struct {
	HTTPAdvertiseAddr   net.TCPAddr
	HTTPBindAddr        net.TCPAddr
	GossipAdvertiseAddr net.TCPAddr
	GossipBindAddr      net.TCPAddr
	Peers               []string
	ReplicationFactor   int
}

type Cluster interface {
	HashRing() hashring.HashRing
	LocalNode() *Node
	Nodes() Nodes
	NodesByPartitionKey(uint64) Nodes
	ReplicationFactor() int
}
