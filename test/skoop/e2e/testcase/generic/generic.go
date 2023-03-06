// nolint
package generic

import (
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	. "github.com/alibaba/kubeskoop/test/skoop/e2e/framework"
	"github.com/onsi/ginkgo/v2"
)

var testSpecs = []*TestSpec{
	{
		Name: "pod to pod",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID: "pod1",
					},
				},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{
						ID:     "pod2",
						Listen: 80,
					},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1", "node2", "pod2"},
			NoSuspicion: true,
			Suspicions:  []AssertionSuspicion{},
			Actions: []AssertionAction{
				{"pod1", model.ActionTypeSend},
				{"node1", model.ActionTypeForward},
				{"node2", model.ActionTypeForward},
				{"pod2", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "pod to node",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID: "pod1",
					},
				},
			},
			{
				ID:     "node2",
				Listen: 80,
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "node2", 80, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1", "node2"},
			NoSuspicion: true,
			Actions: []AssertionAction{
				{"pod1", model.ActionTypeSend},
				{"node1", model.ActionTypeForward},
				{"node2", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "node to node",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
			},
			{
				ID:     "node2",
				Listen: 80,
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("node1", "node2", 80, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"node1", "node2"},
			NoSuspicion: true,
			Actions: []AssertionAction{
				{"node1", model.ActionTypeSend},
				{"node2", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "pod to pod, with pod2 eth0 down",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID: "pod1",
					},
				},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{
						ID:       "pod2",
						Commands: []string{"ip link set eth0 down"},
						Listen:   80,
					},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		Assertion: Assertion{
			Succeed: true,
			Nodes:   []string{"pod1", "node1", "node2", "pod2"},
			Suspicions: []AssertionSuspicion{
				{"pod2", model.SuspicionLevelFatal, "eth0"},
			},
			Actions: []AssertionAction{
				{"pod1", model.ActionTypeSend},
				{"node1", model.ActionTypeForward},
				{"node2", model.ActionTypeForward},
				{"pod2", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "pod to pod, with pod2 lo down",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID: "pod1",
					},
				},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{
						ID:       "pod2",
						Commands: []string{"ip link set lo down"},
						Listen:   80,
					},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		Assertion: Assertion{
			Succeed: true,
			Nodes:   []string{"pod1", "node1", "node2", "pod2"},
			Suspicions: []AssertionSuspicion{
				{"pod2", model.SuspicionLevelFatal, "lo"},
			},
			Actions: []AssertionAction{
				{"pod1", model.ActionTypeSend},
				{"node1", model.ActionTypeForward},
				{"node2", model.ActionTypeForward},
				{"pod2", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "pod to pod, no listen port",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID: "pod1",
					},
				},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{
						ID: "pod2",
					},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		Assertion: Assertion{
			Succeed: true,
			Nodes:   []string{"pod1", "node1", "node2", "pod2"},
			Suspicions: []AssertionSuspicion{
				{"pod2", model.SuspicionLevelFatal, "0.0.0.0:80"},
			},
			Actions: []AssertionAction{
				{"pod1", model.ActionTypeSend},
				{"node1", model.ActionTypeForward},
				{"node2", model.ActionTypeForward},
				{"pod2", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "pod to pod, on same node",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID: "pod1",
					},
					{
						ID:     "pod2",
						Listen: 80,
					},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1", "pod2"},
			NoSuspicion: true,
			Actions: []AssertionAction{
				{"pod1", model.ActionTypeSend},
				{"node1", model.ActionTypeForward},
				{"pod2", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "pod to its node",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID: "pod1",
					},
				},
				Listen: 80,
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "node1", 80, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1"},
			NoSuspicion: true,
			Actions: []AssertionAction{
				{"pod1", model.ActionTypeSend},
				{"node1", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "node to its pod",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID:     "pod1",
						Listen: 80,
					},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("node1", "pod1", 80, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"node1", "pod1"},
			NoSuspicion: true,
			Actions: []AssertionAction{
				{"node1", model.ActionTypeSend},
				{"pod1", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "pod to pod with udp port",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID: "pod1",
					},
					{
						ID:             "pod2",
						Listen:         53,
						ListenProtocol: model.UDP,
					},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 53, model.UDP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1", "pod2"},
			NoSuspicion: true,
			Actions: []AssertionAction{
				{"pod1", model.ActionTypeSend},
				{"node1", model.ActionTypeForward},
				{"pod2", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "node to pod, delete all pod routes",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID:       "pod1",
						Listen:   80,
						Commands: []string{"ip route flush table main"},
					},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("node1", "pod1", 80, model.TCP),
		Assertion: Assertion{
			Succeed: true,
			Nodes:   []string{"node1", "pod1"},
			Suspicions: []AssertionSuspicion{
				{"pod1", model.SuspicionLevelFatal, "no route to host"},
			},
			Actions: []AssertionAction{
				{"node1", model.ActionTypeSend},
			},
		},
	},
	{
		Name: "pod to node, delete all pod routes",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID:       "pod1",
						Listen:   80,
						Commands: []string{"ip route flush table main"},
					},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "node1", 80, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1"},
			NoSuspicion: false,
			Suspicions: []AssertionSuspicion{
				{"pod1", model.SuspicionLevelFatal, "no route to host"},
			},
		},
	},
	{
		Name: "pod to node, wrong route dev",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID: "pod1",
						Commands: []string{
							"ip route flush table main",
							"ip route add default dev lo",
						},
					},
				},
				Listen: 80,
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "node1", 80, model.TCP),
		Assertion: Assertion{
			Succeed: true,
			Nodes:   []string{"pod1", "node1"},
			Suspicions: []AssertionSuspicion{
				{"pod1", model.SuspicionLevelFatal, "route"},
			},
			Actions: []AssertionAction{
				{"pod1", model.ActionTypeSend},
				{"node1", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "pod to pod, pod2 iptables drop all",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID: "pod1",
					},
					{
						ID:       "pod2",
						Listen:   80,
						Commands: []string{"iptables -I INPUT -j DROP"},
					},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1", "pod2"},
			NoSuspicion: false,
			Suspicions: []AssertionSuspicion{
				{"pod2", model.SuspicionLevelFatal, "iptables"},
			},
			Actions: []AssertionAction{
				{"pod1", model.ActionTypeSend},
				{"node1", model.ActionTypeForward},
				{"pod2", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "pod to external",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{ID: "pod1"},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "123.123.123.123", 80, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1", "123.123.123.123"},
			NoSuspicion: true,
			Actions: []AssertionAction{
				{"pod1", model.ActionTypeSend},
				{"node1", model.ActionTypeForward},
				{"123.123.123.123", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "external to pod, will fail",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{ID: "pod1"},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("123.123.123.123", "pod1", 80, model.TCP),
		Assertion: Assertion{
			Succeed: false,
		},
	},
}

var serviceTestSpecs = []*TestSpec{
	{
		Name: "pod to service, with two endpoints",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{ID: "pod1"},
					{ID: "pod2", Listen: 80},
				},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{ID: "pod3", Listen: 80},
				},
			},
		},
		ServiceSpecs: []*ServiceSpec{
			{
				ID:         "service1",
				Endpoints:  []string{"pod2", "pod3"},
				Port:       81,
				TargetPort: 80,
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "service1", 81, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1", "pod2", "node2", "pod3"},
			NoSuspicion: true,
			Actions: []AssertionAction{
				{"pod1", model.ActionTypeSend},
				{"node1", model.ActionTypeService},
				{"pod2", model.ActionTypeServe},
				{"node2", model.ActionTypeForward},
				{"pod3", model.ActionTypeServe},
			},
		},
	},
	{
		Name: "pod to service, with no endpoint",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{ID: "pod1"},
				},
			},
		},
		ServiceSpecs: []*ServiceSpec{
			{ID: "service1", Port: 80, TargetPort: 80},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "service1", 80, model.TCP),
		Assertion: Assertion{
			Succeed: true,
			Nodes:   []string{"pod1", "node1"},
			Suspicions: []AssertionSuspicion{
				{"node1", model.SuspicionLevelFatal, "has no valid endpoint"},
			},
			Actions: []AssertionAction{
				{"pod1", model.ActionTypeSend},
			},
		},
	},
	{
		Name: "node to service, with no endpoint",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
			},
		},
		ServiceSpecs: []*ServiceSpec{
			{ID: "service1", Port: 80, TargetPort: 80},
		},
		DiagnoseSpec: NewDiagnoseSpec("node1", "service1", 80, model.TCP),
		Assertion: Assertion{
			Succeed: true,
			Nodes:   []string{"node1"},
			Suspicions: []AssertionSuspicion{
				{"node1", model.SuspicionLevelFatal, "has no valid endpoint"},
			},
		},
	},
}

func AddGenericTestCases(f *Framework) {
	ginkgo.Describe("generic test cases", func() {
		GenerateTestCases(f, testSpecs)
	})
}

func AddGenericServiceTestCases(f *Framework) {
	ginkgo.Describe("generic service test cases", func() {
		GenerateTestCases(f, serviceTestSpecs)
	})
}
