package model

import (
	"github.com/samber/lo"
)

type NetNodeType string

const (
	NetNodeTypePod      = "pod"
	NetNodeTypeNode     = "node"
	NetNodeTypeExternal = "external"
	NetNodeTypeGeneric  = "generic"
)

type NetNode struct {
	Type       NetNodeType
	ID         string
	Actions    map[*Link]*Action
	Suspicions []Suspicion
	initiative *Action
}

type NetNodeAction interface {
	Send(dst Endpoint, protocol Protocol) ([]Transmission, error)
	Receive(upstream *Link) ([]Transmission, error)
}

func NewNetNode(id string, nodeType NetNodeType) *NetNode {
	return &NetNode{
		ID:      id,
		Type:    nodeType,
		Actions: make(map[*Link]*Action),
	}
}

func (n *NetNode) DoAction(action *Action) {
	if action.Input == nil {
		n.initiative = action
		return
	}
	n.Actions[action.Input] = action
}

func (n *NetNode) ActionOf(input *Link) *Action {
	if input == nil {
		return n.initiative
	}

	return n.Actions[input]
}

func (n *NetNode) AddSuspicion(level SuspicionLevel, msg string) {
	n.Suspicions = append(n.Suspicions, Suspicion{
		Level:   level,
		Message: msg,
	})
}

func (n *NetNode) GetID() string {
	return n.ID
}

func (n *NetNode) GetType() NetNodeType {
	return n.Type
}

func (n *NetNode) GetSuspicions() []Suspicion {
	return n.Suspicions
}

func (n *NetNode) MaxSuspicionLevel() SuspicionLevel {
	if len(n.Suspicions) == 0 {
		return SuspicionLevelInfo
	}
	levels := lo.Map(n.Suspicions, func(s Suspicion, _ int) SuspicionLevel { return s.Level })
	return lo.Max(levels)
}
