package graph

import (
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"strconv"
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
}

type FlowGraph struct {
	Nodes map[string]*Node `json:"nodes"`
	Edges map[string]*Edge `json:"edges"`
}

type podInfo struct {
	Name      string
	Namespace string
	NodeName  string
	IP        string
}

type nodeInfo struct {
	NodeName string
	IP       string
}

func NewFlowGraph() *FlowGraph {
	return &FlowGraph{
		Nodes: make(map[string]*Node),
		Edges: make(map[string]*Edge),
	}
}

func toPodInfo(m model.Vector) (map[string]podInfo, error) {
	ret := make(map[string]podInfo)
	for _, m := range m {
		if _, ok := ret[string(m.Metric["ip"])]; ok {
			continue
		}
		ret[string(m.Metric["ip"])] = podInfo{
			Name:      string(m.Metric["pod_name"]),
			Namespace: string(m.Metric["pod_namespace"]),
			NodeName:  string(m.Metric["node_name"]),
			IP:        string(m.Metric["ip"]),
		}
	}
	return ret, nil
}

func toNodeInfo(m model.Vector) (map[string]nodeInfo, error) {
	ret := make(map[string]nodeInfo)
	for _, m := range m {
		if _, ok := ret[string(m.Metric["ip"])]; ok {
			continue
		}
		ret[string(m.Metric["ip"])] = nodeInfo{
			NodeName: string(m.Metric["node_name"]),
			IP:       string(m.Metric["ip"]),
		}
	}
	return ret, nil
}

func createNode(ip string, podInfo map[string]podInfo, nodeInfo map[string]nodeInfo) Node {
	n := Node{
		ID: ip,
		IP: ip,
	}

	if i, ok := podInfo[ip]; ok {
		n.Type = "pod"
		n.Name = i.Name
		n.Namespace = i.Namespace
		n.NodeName = i.NodeName
	} else if i, ok := nodeInfo[ip]; ok {
		n.Type = "node"
		n.NodeName = i.NodeName
	} else {
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

func createEdge(src, dst string, v *model.Sample) Edge {
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

func FromVector(m model.Vector, podInfo model.Vector, nodeInfo model.Vector) (*FlowGraph, error) {
	g := NewFlowGraph()
	pi, err := toPodInfo(podInfo)
	if err != nil {
		return nil, err
	}
	ni, err := toNodeInfo(nodeInfo)
	if err != nil {
		return nil, err
	}
	for _, v := range m {
		src := string(v.Metric["src"])
		dst := string(v.Metric["dst"])
		g.AddNode(createNode(src, pi, ni))
		g.AddNode(createNode(dst, pi, ni))
		g.AddEdge(createEdge(src, dst, v))
	}
	return g, nil
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
		if _, ok := g.Edges[id]; ok {
			g.Edges[id].Bytes = int(v.Value)
		}
	}
}

func (g *FlowGraph) SetEdgePacketsFromVector(m model.Vector) {
	for _, v := range m {
		id := getEdgeID(v)
		if _, ok := g.Edges[id]; ok {
			g.Edges[id].Packets = int(v.Value)
		}
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
