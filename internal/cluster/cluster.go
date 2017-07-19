package cluster

import (
	"fmt"
	"io/ioutil"
	"net"
	"strconv"

	"github.com/hashicorp/memberlist"
)

var c = struct {
	d  *delegate
	ml *memberlist.Memberlist
}{
	d: &delegate{},
}

type Config struct {
	AdvertiseAddr string
	BindAddr      string
	Peers         []string
}

func Join(config *Config) error {
	// FIXME(mbostock): Consider using a non-local config for memberlist
	memberConf := memberlist.DefaultLocalConfig()
	advHost, advPort, _ := net.SplitHostPort(config.BindAddr)
	bindPort, _ := strconv.Atoi(advPort)
	memberConf.BindAddr = advHost
	memberConf.BindPort = bindPort
	memberConf.LogOutput = ioutil.Discard

	var err error
	if c.ml, err = memberlist.Create(memberConf); err != nil {
		return fmt.Errorf("Failed to configure cluster settings: %s", err)
	}
	c.ml.Join(config.Peers)
	return nil
}

type Node struct {
	mln *memberlist.Node
}

func (n *Node) Name() string {
	return n.mln.Name
}
func (n *Node) Addr() string {
	return fmt.Sprintf("%s:%d", n.mln.Addr.String(), n.mln.Port)
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

func Nodes() (nodes []*Node) {
	if c.ml == nil {
		panic("Not yet joined a cluster")
	}
	for _, n := range c.ml.Members() {
		nodes = append(nodes, &Node{n})
	}
	return
}

type delegate struct{}

func (d *delegate) NodeMeta(limit int) []byte {
	panic("not implemented")
}

func (d *delegate) NotifyMsg([]byte) {
	panic("not implemented")
}

func (d *delegate) GetBroadcasts(overhead int, limit int) [][]byte {
	panic("not implemented")
}

func (d *delegate) LocalState(join bool) []byte {
	panic("not implemented")
}

func (d *delegate) MergeRemoteState(buf []byte, join bool) {
	panic("not implemented")
}
