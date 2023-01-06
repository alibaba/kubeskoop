// nolint
package plugin

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	. "github.com/alibaba/kubeskoop/test/skoop/e2e/framework"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	"github.com/projectcalico/api/pkg/client/clientset_generated/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type calicoIPPoolSpec struct {
	Name        string
	IPIPMode    calicov3.IPIPMode
	CIDR        string
	NATOutgoing bool
}

type calicoExtraSpec struct {
	IPPools []calicoIPPoolSpec
}

func calicoIPPoolAnnotation(ippool string) map[string]string {
	return map[string]string{
		"cni.projectcalico.org/ipv4pools": fmt.Sprintf("[\"%s\"]", ippool),
	}
}

func createCalicoIPPoolFromSpec(f *Framework, client *clientset.Clientset, spec *calicoIPPoolSpec) error {
	pool := &calicov3.IPPool{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "projectcalico.org/v3",
			Kind:       "IPPool",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: E2ENamespace,
			Labels: map[string]string{
				"e2e": "yes",
			},
		},
		Spec: calicov3.IPPoolSpec{
			CIDR:         spec.CIDR,
			IPIPMode:     spec.IPIPMode,
			NATOutgoing:  spec.NATOutgoing,
			NodeSelector: "!all()",
		},
	}

	pool, err := client.ProjectcalicoV3().IPPools().Create(context.TODO(), pool, metav1.CreateOptions{})
	return err
}

func clearCalicoIPPools(client *clientset.Clientset) error {
	return client.ProjectcalicoV3().IPPools().DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
			MatchLabels: map[string]string{
				"e2e": "yes",
			},
		}),
	})
}

var calicoTestSpecs = []*TestSpec{
	{
		Name: "pod to pod, bgp mode",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{ID: "pod1", Annotations: calicoIPPoolAnnotation("ippool1")},
				},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{ID: "pod2", Listen: 80, Annotations: calicoIPPoolAnnotation("ippool1")},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		ExtraSpec: calicoExtraSpec{
			IPPools: []calicoIPPoolSpec{
				{
					Name:     "ippool1",
					CIDR:     "10.245.0.0/16",
					IPIPMode: calicov3.IPIPModeNever,
				},
			},
		},
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1", "node2", "pod2"},
			NoSuspicion: true,
		},
	},
	{
		Name: "pod to pod, bgp mode node veth peer down",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{ID: "pod1", Annotations: calicoIPPoolAnnotation("ippool1")},
				},
				Commands:        []string{"ip link set {{.Pod.pod1.Extra.veth}} down"},
				RecoverCommands: []string{"ip link set {{.Pod.pod1.Extra.veth}} up"},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{ID: "pod2", Listen: 80, Annotations: calicoIPPoolAnnotation("ippool1")},
				},
				Commands:        []string{"ip link set {{.Pod.pod2.Extra.veth}} down"},
				RecoverCommands: []string{"ip link set {{.Pod.pod2.Extra.veth}} up"},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		ExtraSpec: calicoExtraSpec{
			IPPools: []calicoIPPoolSpec{
				{
					Name:     "ippool1",
					CIDR:     "10.245.0.0/16",
					IPIPMode: calicov3.IPIPModeNever,
				},
			},
		},
		Assertion: Assertion{
			Succeed: true,
			Nodes:   []string{"pod1", "node1", "node2", "pod2"},
			Suspicions: []AssertionSuspicion{
				{"node1", model.SuspicionLevelFatal, "State is not expect"},
				{"node2", model.SuspicionLevelFatal, "State is not expect"},
			},
		},
	},
	{
		Name: "pod to pod, bgp mode delete pod route",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{ID: "pod1", Annotations: calicoIPPoolAnnotation("ippool1")},
				},
				Commands: []string{"ip route del {{.Pod.pod1.IP}}"},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{ID: "pod2", Listen: 80, Annotations: calicoIPPoolAnnotation("ippool1")},
				},
				Commands: []string{"ip route del {{.Pod.pod2.IP}}"},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		ExtraSpec: calicoExtraSpec{
			IPPools: []calicoIPPoolSpec{
				{
					Name:     "ippool1",
					CIDR:     "10.245.0.0/16",
					IPIPMode: calicov3.IPIPModeNever,
				},
			},
		},
		Assertion: Assertion{
			Succeed: true,
			Nodes:   []string{"pod1", "node1", "node2", "pod2"},
			Suspicions: []AssertionSuspicion{
				{"node1", model.SuspicionLevelFatal, "route"},
				{"node2", model.SuspicionLevelFatal, "route"},
			},
		},
	},
	{
		Name: "pod to pod, ipip mode",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{ID: "pod1", Annotations: calicoIPPoolAnnotation("ippool1")},
				},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{ID: "pod2", Listen: 80, Annotations: calicoIPPoolAnnotation("ippool1")},
				},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		ExtraSpec: calicoExtraSpec{
			IPPools: []calicoIPPoolSpec{
				{
					Name:     "ippool1",
					CIDR:     "10.245.0.0/16",
					IPIPMode: calicov3.IPIPModeAlways,
				},
			},
		},
		Assertion: Assertion{
			Succeed:     true,
			Nodes:       []string{"pod1", "node1", "node2", "pod2"},
			NoSuspicion: true,
		},
	},
	{
		Name: "pod to pod, ipip mode, wrong route",
		NodeSpecs: []*NodeSpec{
			{
				ID: "node1",
				PodSpecs: []*PodSpec{
					{ID: "pod1", Annotations: calicoIPPoolAnnotation("ippool1")},
				},
				Commands:        []string{"ip route add {{.Pod.pod2.IP}}/32 dev eth0"},
				RecoverCommands: []string{"ip route del {{.Pod.pod2.IP}}/32"},
			},
			{
				ID: "node2",
				PodSpecs: []*PodSpec{
					{ID: "pod2", Listen: 80, Annotations: calicoIPPoolAnnotation("ippool1")},
				},
				Commands:        []string{"ip route add {{.Pod.pod1.IP}}/32 dev eth0"},
				RecoverCommands: []string{"ip route del {{.Pod.pod1.IP}}/32"},
			},
		},
		DiagnoseSpec: NewDiagnoseSpec("pod1", "pod2", 80, model.TCP),
		ExtraSpec: calicoExtraSpec{
			IPPools: []calicoIPPoolSpec{
				{
					Name:     "ippool1",
					CIDR:     "10.245.0.0/16",
					IPIPMode: calicov3.IPIPModeAlways,
				},
			},
		},
		Assertion: Assertion{
			Succeed: true,
			Nodes:   []string{"pod1", "node1", "node2", "pod2"},
			Suspicions: []AssertionSuspicion{
				{"node1", model.SuspicionLevelFatal, "route"},
				{"node2", model.SuspicionLevelFatal, "route"},
			},
		},
	},
}

const calicoDefaultInterfacePrefix = "cali"

func calicoVethName(namespace, name string) string {
	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%s.%s", namespace, name)))
	return fmt.Sprintf("%s%s", calicoDefaultInterfacePrefix, hex.EncodeToString(h.Sum(nil))[:11])
}

func AddCalicoTestSpecs(f *Framework) {
	ginkgo.Describe("calico", func() {
		client, err := clientset.NewForConfig(f.RestConfig())
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		ginkgo.AfterEach(func() {
			ginkgo.By("Recover")
			//err := f.Recover()
			//gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		for _, t := range calicoTestSpecs {
			t := t
			ginkgo.It(t.Name, func() {
				f.SetTestSpec(t)

				ginkgo.By("Assign nodes")
				err := f.AssignNodes()
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				for _, n := range t.NodeSpecs {
					for _, p := range n.PodSpecs {
						vethName := calicoVethName(E2ENamespace, fmt.Sprintf("e2e-pod-%s", p.ID))
						p.ExtraInfo = map[string]string{
							"veth": vethName,
						}
					}
				}

				if extraSpec, ok := t.ExtraSpec.(calicoExtraSpec); ok {
					if len(extraSpec.IPPools) != 0 {
						err = clearCalicoIPPools(client)
						gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
						for _, p := range extraSpec.IPPools {
							err = createCalicoIPPoolFromSpec(f, client, &p)
							gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
						}
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
