package cluster

import "github.com/hashicorp/memberlist"

type membership struct{ l *memberlist.Memberlist }

func (m *membership) Nodes() Nodes {
	nodes := make(Nodes, 0, len(m.l.Members()))
	for _, n := range m.l.Members() {
		nodes = append(nodes, &Node{n})
	}
	return nodes

}

func (m *membership) LocalNode() *Node {
	return &Node{m.l.LocalNode()}
}

type Membership interface {
	LocalNode() *Node
	Nodes() Nodes
}
