// nolint
package plugin

import (
	"net"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	. "github.com/alibaba/kubeskoop/test/skoop/e2e/framework"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var flannelHostGWTestSpecs = []*TestSpec{
	{
		Name: "pod to pod, pod mtu set to 1400",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID:       "pod1",
						Commands: []string{"ip link set dev eth0 mtu 1400"},
					},
				},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{
						ID:       "pod2",
						Commands: []string{"ip link set dev eth0 mtu 1400"},
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
				{"pod1", model.SuspicionLevelFatal, "MTU"},
				{"pod2", model.SuspicionLevelFatal, "MTU"},
			},
		},
	},
	{
		Name: "pod to pod, pod1 node cni0 down",
		NodeSpecs: []*NodeSpec{
			{
				ID:              "node1",
				Commands:        []string{"ip link set dev cni0 down"},
				RecoverCommands: []string{"ip link set dev cni0 up"},
				PodSpecs: []*PodSpec{
					{ID: "pod1"},
				},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{ID: "pod2", Listen: 80},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1", "node2", "pod2"},
			NoSuspicion: false,
			Suspicions: []AssertionSuspicion{
				{"node1", model.SuspicionLevelFatal, "cni0"},
			},
		},
	},
	{
		Name: "pod to pod, delete pod cidr to cni0 route",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{ID: "pod1"},
				},
				Commands:        []string{"ip route del {{.Node.node1.PodCIDR}}"},
				RecoverCommands: []string{"ip route add {{.Node.node1.PodCIDR}} dev cni0 scope link"},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{ID: "pod2", Listen: 80},
				},
				Commands:        []string{"ip route del {{.Node.node2.PodCIDR}}"},
				RecoverCommands: []string{"ip route add {{.Node.node2.PodCIDR}} dev cni0 scope link"},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		Assertion: Assertion{
			Succeed: true,
			Nodes:   []string{"pod1", "node1", "node2", "pod2"},
			Suspicions: []AssertionSuspicion{
				{"node1", model.SuspicionLevelFatal, "invalid route"},
				{"node2", model.SuspicionLevelFatal, "invalid route"},
			},
		},
	},
	{
		Name: "pod to pod, set wrong routes to nodes",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{ID: "pod1"},
				},
				Commands: []string{
					"ip route del {{.Node.node2.PodCIDR}}",
					"ip route add {{.Node.node2.PodCIDR}} via {{.Node.node1.IP}} dev eth0",
				},
				RecoverCommands: []string{
					"ip route del {{.Node.node2.PodCIDR}}",
					"ip route add {{.Node.node2.PodCIDR}} via {{.Node.node2.IP}} dev eth0",
				},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{ID: "pod2", Listen: 80},
				},
				Commands: []string{
					"ip route del {{.Node.node1.PodCIDR}}",
					"ip route add {{.Node.node1.PodCIDR}} via {{.Node.node2.IP}} dev eth0",
				},
				RecoverCommands: []string{
					"ip route del {{.Node.node1.PodCIDR}}",
					"ip route add {{.Node.node1.PodCIDR}} via {{.Node.node1.IP}} dev eth0",
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		Assertion: Assertion{
			Succeed: true,
			Nodes:   []string{"pod1", "node1", "node2", "pod2"},
			Suspicions: []AssertionSuspicion{
				{"node1", model.SuspicionLevelFatal, "invalid route"},
				{"node2", model.SuspicionLevelFatal, "invalid route"},
			},
		},
	},
}

func AddFlannelHostGwTestCases(f *Framework) {
	ginkgo.Describe("flannel host-gw", func() {
		GenerateTestCases(f, flannelHostGWTestSpecs)
	})
}

var flannelVxlanTestSpecs = []*TestSpec{
	{
		Name: "pod to pod, pod mtu set to 1400",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{
						ID:       "pod1",
						Commands: []string{"ip link set dev eth0 mtu 1400"},
					},
				},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{
						ID:       "pod2",
						Commands: []string{"ip link set dev eth0 mtu 1400"},
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
				{"pod1", model.SuspicionLevelFatal, "MTU"},
				{"pod2", model.SuspicionLevelFatal, "MTU"},
			},
		},
	},
	//{
	//	Name: "pod to pod, node flannel.1 down",
	//	NodeSpecs: []*NodeSpec{
	//		{
	//			ID: "node1",
	//			PodSpecs: []*PodSpec{
	//				{ID: "pod1"},
	//			},
	//			Commands:        []string{"ip link set dev flannel.1 down"},
	//			RecoverCommands: []string{"ip link set dev flannel.1 up"},
	//		},
	//		{
	//			ID: "node2",
	//			PodSpecs: []*PodSpec{
	//				{ID: "pod2"},
	//			},
	//			Commands:        []string{"ip link set dev flannel.1 down"},
	//			RecoverCommands: []string{"ip link set dev flannel.1 up"},
	//			Listen:          80,
	//		},
	//	},
	//	DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
	//	Assertion: Assertion{
	//		Succeed: true,
	//		Nodes:   []string{"pod1", "node1", "node2", "pod2"},
	//		Suspicions: []AssertionSuspicion{
	//			{"node1", model.SuspicionLevelFatal, "flannel.1"},
	//			{"node2", model.SuspicionLevelFatal, "flannel.1"},
	//		},
	//	},
	//},
	{
		Name: "pod to pod, pod1 node cni0 down",
		NodeSpecs: []*NodeSpec{
			{
				ID:              "node1",
				Commands:        []string{"ip link set dev cni0 down"},
				RecoverCommands: []string{"ip link set dev cni0 up"},
				PodSpecs: []*PodSpec{
					{ID: "pod1"},
				},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{ID: "pod2", Listen: 80},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1", "node2", "pod2"},
			NoSuspicion: false,
			Suspicions: []AssertionSuspicion{
				{"node1", model.SuspicionLevelFatal, "cni0"},
			},
		},
	},
	{
		Name: "pod to pod, node delete pod cidr route to flannel.1",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{ID: "pod1"},
				},
				Commands:        []string{"ip route del {{.Node.node2.PodCIDR}}"},
				RecoverCommands: []string{"ip route add {{.Node.node2.PodCIDR}} via {{.Node.node2.Extra.gateway}} dev flannel.1 onlink"},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{ID: "pod2", Listen: 80},
				},
				Commands:        []string{"ip route del {{.Node.node1.PodCIDR}}"},
				RecoverCommands: []string{"ip route add {{.Node.node1.PodCIDR}} via {{.Node.node1.Extra.gateway}} dev flannel.1 onlink"},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1", "node2", "pod2"},
			NoSuspicion: false,
			Suspicions: []AssertionSuspicion{
				{"node1", model.SuspicionLevelFatal, "route"},
				{"node2", model.SuspicionLevelFatal, "route"},
			},
		},
	},
	{
		Name: "pod to pod, wrong pod cidr route",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{ID: "pod1"},
				},
				Commands:        []string{"ip route del {{.Node.node2.PodCIDR}}", "ip route add {{.Node.node2.PodCIDR}} dev eth0"},
				RecoverCommands: []string{"ip route del {{.Node.node2.PodCIDR}}", "ip route add {{.Node.node2.PodCIDR}} via {{.Node.node2.Extra.gateway}} dev flannel.1 onlink"},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{ID: "pod2", Listen: 80},
				},
				Commands:        []string{"ip route del {{.Node.node1.PodCIDR}}", "ip route add {{.Node.node1.PodCIDR}} dev eth0"},
				RecoverCommands: []string{"ip route del {{.Node.node1.PodCIDR}}", "ip route add {{.Node.node1.PodCIDR}} via {{.Node.node1.Extra.gateway}} dev flannel.1 onlink"},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1", "node2", "pod2"},
			NoSuspicion: false,
			Suspicions: []AssertionSuspicion{
				{"node1", model.SuspicionLevelFatal, "route"},
				{"node2", model.SuspicionLevelFatal, "route"},
			},
		},
	},
}

func AddFlannelVxlanTestCases(f *Framework) {
	ginkgo.Describe("flannel vxlan", func() {
		ginkgo.AfterEach(func() {
			ginkgo.By("Recover")
			err := f.Recover()
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		for _, t := range flannelVxlanTestSpecs {
			t := t
			ginkgo.It(t.Name, func() {
				f.SetTestSpec(t)

				ginkgo.By("Assign nodes")
				err := f.AssignNodes()
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				// set gateway to extra data
				for _, n := range t.NodeSpecs {
					node := f.Node(n.ID)
					gomega.Expect(node).NotTo(gomega.BeNil())
					ip, _, err := net.ParseCIDR(node.Spec.PodCIDR)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
					n.ExtraInfo = map[string]string{
						"gateway": ip.String(),
					}
				}

				ginkgo.By("Prepare")
				err = f.Prepare()
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				ginkgo.By("Run diagnose")
				err = f.RunDiagnose()
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				ginkgo.By("Assert")
				err = f.Assert()
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			})
		}
	})
}
