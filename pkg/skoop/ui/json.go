package ui

import (
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"k8s.io/apimachinery/pkg/util/json"
)

type DiagnoseSummarySuspicion struct {
	Level   model.SuspicionLevel `json:"level"`
	Message string               `json:"message"`
}

type DiagnoseSummaryNodeAction struct {
	Type model.ActionType `json:"type"`
}

type DiagnoseSummaryNode struct {
	ID         string                               `json:"id"`
	Type       model.NetNodeType                    `json:"type"`
	Suspicions []DiagnoseSummarySuspicion           `json:"suspicions"`
	Actions    map[string]DiagnoseSummaryNodeAction `json:"actions"`
}

type DiagnoseSummaryPacket struct {
	Src      string         `json:"source"`
	Dst      string         `json:"destination"`
	Dport    uint16         `json:"dport"`
	Protocol model.Protocol `json:"protocol"`
}

type DiagnoseSummaryLink struct {
	ID                   string                `json:"id"`
	Source               string                `json:"source"`
	SourceAttribute      map[string]string     `json:"source_attributes"`
	DestinationAttribute map[string]string     `json:"destination_attributes"`
	Destination          string                `json:"destination"`
	Type                 model.LinkType        `json:"type"`
	ActionType           model.ActionType      `json:"action"`
	Packet               DiagnoseSummaryPacket `json:"packet"`
}

type DiagnoseSummaryCluster struct {
	Suspicions []DiagnoseSummarySuspicion `json:"suspicions"`
}

type DiagnoseSummary struct {
	Cluster DiagnoseSummaryCluster `json:"cluster"`
	Nodes   []DiagnoseSummaryNode  `json:"nodes"`
	Links   []DiagnoseSummaryLink  `json:"links"`
}

type JSONFormatter struct {
	globalSuspicions []model.Suspicion
	p                *model.PacketPath
}

func NewJSONFormatter(globalSuspicions []model.Suspicion, p *model.PacketPath) *JSONFormatter {
	return &JSONFormatter{globalSuspicions: globalSuspicions, p: p}
}

func (f *JSONFormatter) ToJSON() ([]byte, error) {
	summary, err := f.toSummary()
	if err != nil {
		return nil, err
	}

	return json.Marshal(&summary)
}

func (f *JSONFormatter) toSummary() (*DiagnoseSummary, error) {
	summary := &DiagnoseSummary{
		Cluster: DiagnoseSummaryCluster{Suspicions: []DiagnoseSummarySuspicion{}},
	}

	for _, sus := range f.globalSuspicions {
		summary.Cluster.Suspicions = append(summary.Cluster.Suspicions, DiagnoseSummarySuspicion{
			Level:   sus.Level,
			Message: sus.Message,
		})
	}

	for _, node := range f.p.Nodes() {
		n := DiagnoseSummaryNode{
			ID:         node.GetID(),
			Type:       node.GetType(),
			Suspicions: nil,
			Actions:    map[string]DiagnoseSummaryNodeAction{},
		}

		for _, suspicion := range node.GetSuspicions() {
			n.Suspicions = append(n.Suspicions, DiagnoseSummarySuspicion{
				Level:   suspicion.Level,
				Message: suspicion.Message,
			})
		}

		for link, action := range node.Actions {
			n.Actions[link.GetID()] = DiagnoseSummaryNodeAction{
				Type: action.Type,
			}
		}

		initiative := node.ActionOf(nil)
		if initiative != nil {
			n.Actions[""] = DiagnoseSummaryNodeAction{
				Type: initiative.Type,
			}
		}

		summary.Nodes = append(summary.Nodes, n)
	}

	for _, link := range f.p.Links() {
		l := DiagnoseSummaryLink{
			ID:          link.GetID(),
			Type:        link.Type,
			Destination: link.Destination.GetID(),
			Packet: DiagnoseSummaryPacket{
				Src:      link.Packet.Src.String(),
				Dst:      link.Packet.Dst.String(),
				Dport:    link.Packet.Dport,
				Protocol: link.Packet.Protocol,
				// todo: encapped packet
			},
		}

		if link.Source != nil {
			l.Source = link.Source.GetID()
		}

		if link.SourceAttribute != nil {
			l.SourceAttribute = link.SourceAttribute.GetAttrs()
		}

		if link.DestinationAttribute != nil {
			l.DestinationAttribute = link.DestinationAttribute.GetAttrs()
		}

		summary.Links = append(summary.Links, l)
	}

	return summary, nil
}
