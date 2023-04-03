package ui

import (
	"bytes"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"

	graphviz "github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
)

const (
	GraphvizFormatDot = string(graphviz.XDOT)
	GraphvizFormatSvg = string(graphviz.SVG)
)

type Graphviz struct {
	g     *graphviz.Graphviz
	graph *cgraph.Graph
}

func NewGraphviz(p *model.PacketPath) (*Graphviz, error) {
	g := &Graphviz{
		g: graphviz.New(),
	}

	graph, err := g.buildGraph(p)
	if err != nil {
		return nil, err
	}

	graph.SetNodeSeparator(2).SetRankDir(cgraph.LRRank)
	g.graph = graph
	return g, nil
}

func trimID(id string) string {
	ids := strings.Split(id, "/")
	return ids[len(ids)-1]
}

func (g *Graphviz) buildGraph(p *model.PacketPath) (*cgraph.Graph, error) {
	graph, err := g.g.Graph()
	if err != nil {
		return nil, err
	}

	getNode := func(netNode *model.NetNode) (*cgraph.Node, error) {
		id := netNode.GetID()
		n, err := graph.Node(id)
		if err != nil {
			return nil, err
		}

		if n != nil {
			return n, nil
		}

		n, err = graph.CreateNode(id)
		if err != nil {
			return nil, err
		}

		n.SetID(id).SetLabel(trimID(id))
		n.SetHeight(1.8).SetWidth(1.8).SetShape(cgraph.CircleShape)
		n.SetFontSize(11)

		return n, nil
	}

	_, err = getNode(p.GetOriginNode())
	if err != nil {
		return nil, err
	}

	for _, l := range p.Links() {
		s, err := getNode(l.Source)
		if err != nil {
			return nil, err
		}

		d, err := getNode(l.Destination)
		if err != nil {
			return nil, err
		}

		edge, err := graph.CreateEdge(l.GetID(), s, d)
		if err != nil {
			return nil, err
		}

		action := l.Destination.ActionOf(l)
		edge.SetID(l.GetID()).SetLabel(string(l.Type))

		label := p.GetLinkLabel(l, action)
		edge.Set("linklabels", label)

		if action.Type == model.ActionTypeServe {
			edge.SetArrowHead(cgraph.DotArrow)
		}
	}

	return graph, nil
}

func (g *Graphviz) To(format string) ([]byte, error) {
	buf := bytes.Buffer{}
	err := g.g.Render(g.graph, graphviz.Format(format), &buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (g *Graphviz) ToDot() ([]byte, error) {
	return g.To(GraphvizFormatDot)
}

func (g *Graphviz) ToSvg() ([]byte, error) {
	return g.To(GraphvizFormatSvg)
}
