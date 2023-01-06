package framework

import (
	model2 "github.com/alibaba/kubeskoop/pkg/skoop/model"
	v1 "k8s.io/api/core/v1"
)

type TestSpec struct {
	Name              string
	NodeSpecs         []*NodeSpec
	ServiceSpecs      []*ServiceSpec
	NetworkPolicySpec []*NetworkPolicySpec
	DiagnoseSpec      DiagnoseSpec
	//TODO: provider spec & k8s spec (network policy, service)
	Assertion    Assertion
	ProviderSpec interface{}
	ExtraSpec    interface{}
}

type NodeSpec struct {
	ID              string
	Commands        []string
	RecoverCommands []string // todo: recreate node
	PodSpecs        []*PodSpec
	Listen          uint16
	ListenProtocol  model2.Protocol
	ExtraInfo       map[string]string
	node            *v1.Node
	skoopID         string
}

type PodSpec struct {
	ID             string
	Commands       []string
	Listen         uint16
	ListenProtocol model2.Protocol
	Annotations    map[string]string
	ExtraInfo      map[string]string
	pod            *v1.Pod
	ownerService   []string
	skoopID        string
}

type Assertion struct {
	Succeed     bool
	Nodes       []string
	NoSuspicion bool
	Suspicions  []AssertionSuspicion
	Actions     []AssertionAction
	//TODO: implement link assertion
	//Links       []AssertionLink
}

type AssertionSuspicion struct {
	On       string
	Level    model2.SuspicionLevel
	Contains string
}

type AssertionLink struct {
	From   string
	To     string
	Oif    string
	Iif    string
	Packet *AssertionPacket
}

type AssertionPacket struct {
}

type AssertionAction struct {
	On   string
	Type model2.ActionType
}

type DiagnoseSpec struct {
	From     string
	To       string
	Port     uint16
	Protocol model2.Protocol
}

type ServiceSpec struct {
	ID             string
	Endpoints      []string
	Port           uint16
	TargetPort     uint16
	TargetPortName string
	Protocol       model2.Protocol
	service        *v1.Service
}

type NetworkPolicySpec struct {
}

func NewDiagnoseSpec(from, to string, port uint16, protocol model2.Protocol) DiagnoseSpec {
	return DiagnoseSpec{From: from, To: to, Port: port, Protocol: protocol}
}
