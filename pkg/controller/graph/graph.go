package graph

import (
	"fmt"
	"strconv"

	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
)

type Node struct {
	ID        string `json:"id"`
	IP        string `json:"ip"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	NodeName  string `json:"node_name"`
}
type Edge struct {
	ID       string `json:"id"`
	Src      string `json:"src"`
	Dst      string `json:"dst"`
	Sport    int    `json:"sport"`
	Dport    int    `json:"dport"`
	Protocol string `json:"protocol"`
	Bytes    int    `json:"bytes"`
	Packets  int    `json:"packets"`
	Dropped  int    `json:"dropped"`
	Retrans  int    `json:"retrans"`
}

type FlowGraph struct {
	Nodes map[string]*Node `json:"nodes"`
	Edges map[string]*Edge `json:"edges"`
}

func NewFlowGraph() *FlowGraph {
	return &FlowGraph{
		Nodes: make(map[string]*Node),
		Edges: make(map[string]*Edge),
	}
}

func createNode(t, ip, podNamespace, podName, nodeName string) Node {
	n := Node{
		ID: ip,
		IP: ip,
	}

	switch t {
	case "pod":
		n.Type = "pod"
		n.Name = podName
		n.Namespace = podNamespace
		n.NodeName = nodeName
	case "node":
		n.Type = "node"
		n.NodeName = nodeName
	default:
		n.Type = "external"
	}

	return n
}

func getEdgeID(v *model.Sample) string {
	src := string(v.Metric["src"])
	dst := string(v.Metric["dst"])
	sport, _ := strconv.Atoi(string(v.Metric["sport"]))
	dport, _ := strconv.Atoi(string(v.Metric["dport"]))
	protocol := string(v.Metric["protocol"])
	return fmt.Sprintf("%s-%s:%d-%s:%d",
		protocol, src, sport, dst, dport)
}

func createEdge(v *model.Sample) Edge {
	src := string(v.Metric["src"])
	dst := string(v.Metric["dst"])
	sport, _ := strconv.Atoi(string(v.Metric["sport"]))
	dport, _ := strconv.Atoi(string(v.Metric["dport"]))
	protocol := string(v.Metric["protocol"])
	return Edge{
		ID:       getEdgeID(v),
		Src:      src,
		Dst:      dst,
		Sport:    sport,
		Dport:    dport,
		Protocol: protocol,
	}
}

func FromVector(m model.Vector) (*FlowGraph, error) {
	g := NewFlowGraph()
	for _, v := range m {
		g.AddNodesFromSample(v)
	}
	return g, nil
}

func (g *FlowGraph) AddNodesFromVector(v model.Vector) {
	for _, s := range v {
		g.AddNodesFromSample(s)
	}
}

func (g *FlowGraph) AddNodesFromSample(v *model.Sample) {
	srcIP := string(v.Metric["src"])
	if srcIP != "" {
		t := string(v.Metric["src_type"])
		podName := string(v.Metric["src_pod"])
		podNamespace := string(v.Metric["src_namespace"])
		nodeName := string(v.Metric["src_node"])
		g.AddNode(createNode(t, srcIP, podNamespace, podName, nodeName))
	}

	dstIP := string(v.Metric["dst"])
	if dstIP != "" {
		t := string(v.Metric["dst_type"])
		podName := string(v.Metric["dst_pod"])
		podNamespace := string(v.Metric["dst_namespace"])
		nodeName := string(v.Metric["dst_node"])
		g.AddNode(createNode(t, dstIP, podNamespace, podName, nodeName))
	}
}

func (g *FlowGraph) AddNode(n Node) {
	if _, ok := g.Nodes[n.ID]; !ok {
		g.Nodes[n.ID] = &n
	}
}

func (g *FlowGraph) AddEdge(e Edge) {
	if _, ok := g.Edges[e.ID]; !ok {
		g.Edges[e.ID] = &e
	}
}

func (g *FlowGraph) SetEdgeBytesFromVector(m model.Vector) {
	for _, v := range m {
		id := getEdgeID(v)
		if _, ok := g.Edges[id]; !ok {
			g.AddEdge(createEdge(v))
		}
		g.Edges[id].Bytes = int(v.Value)
	}
}

func (g *FlowGraph) SetEdgePacketsFromVector(m model.Vector) {
	for _, v := range m {
		id := getEdgeID(v)
		if _, ok := g.Edges[id]; !ok {
			g.AddEdge(createEdge(v))
		}
		g.Edges[id].Packets = int(v.Value)
	}
}

func (g *FlowGraph) SetEdgeDroppedFromVector(m model.Vector) {
	for _, v := range m {
		id := getEdgeID(v)
		if _, ok := g.Edges[id]; !ok {
			g.AddEdge(createEdge(v))
		}
		g.Edges[id].Dropped = int(v.Value)
	}
}

func (g *FlowGraph) SetEdgeRetransFromVector(m model.Vector) {
	for _, v := range m {
		id := getEdgeID(v)
		if _, ok := g.Edges[id]; !ok {
			g.AddEdge(createEdge(v))
		}
		g.Edges[id].Retrans = int(v.Value)
	}
}

func (g *FlowGraph) ToJSON() ([]byte, error) {
	ret := struct {
		Nodes []*Node `json:"nodes"`
		Edges []*Edge `json:"edges"`
	}{
		Nodes: lo.Values(g.Nodes),
		Edges: lo.Values(g.Edges),
	}

	return jsoniter.Marshal(&ret)
}
