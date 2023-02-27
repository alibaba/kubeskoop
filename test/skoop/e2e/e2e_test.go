package e2e

import (
	"flag"
	"strings"
	"testing"

	"github.com/alibaba/kubeskoop/pkg/skoop/utils"
	"github.com/alibaba/kubeskoop/test/skoop/e2e/framework"
	"github.com/alibaba/kubeskoop/test/skoop/e2e/testcase"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
)

type options struct {
	KubeConfigPath    string
	CloudProvider     string
	SkoopPath         string
	Testcases         string
	CollectorImage    string
	ExtraDiagnoseArgs string
}

func (o *options) BindFlags() {
	flag.StringVar(&o.KubeConfigPath, "kube-config", "~/.kube/config", "cluster kubeconfig file")
	flag.StringVar(&o.CloudProvider, "cloud-provider", "generic", "cloud provider of cluster")
	flag.StringVar(&o.SkoopPath, "executable", "kubeskoop", "kubeskoop executable file")
	flag.StringVar(&o.Testcases, "testcases", "generic", "testcases for e2e test, separated by comma.")
	flag.StringVar(&o.CollectorImage, "collector-image", "kubeskoop/kubeskoop:v0.1.0", "collector image for skoop cli")
	flag.StringVar(&o.ExtraDiagnoseArgs, "extra-diagnose-args", "", "extra args for skoop")
}

var globalOptions = &options{}

func init() {
	globalOptions.BindFlags()
}

func TestE2E(t *testing.T) {
	restConfig, _, err := utils.NewConfig(globalOptions.KubeConfigPath)
	if err != nil {
		t.Fatalf("error init kubernetes rest client from %s, err: %v", globalOptions.KubeConfigPath, err)
	}

	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("error init kubernetes rest client from %s, err: %v", globalOptions.KubeConfigPath, err)
	}

	var extraDiagnoseArgs []string
	if globalOptions.ExtraDiagnoseArgs != "" {
		extraDiagnoseArgs = strings.Split(globalOptions.ExtraDiagnoseArgs, " ")
	}
	f, err := framework.NewFramework(clientSet, restConfig, globalOptions.SkoopPath, globalOptions.CloudProvider,
		globalOptions.KubeConfigPath, globalOptions.CollectorImage, extraDiagnoseArgs)
	if err != nil {
		t.Fatalf("error create framework: %v", err)
	}

	gomega.RegisterFailHandler(ginkgo.Fail)

	testcases := strings.Split(globalOptions.Testcases, ",")
	ginkgo.Describe("kubeskoop e2e tests", func() {
		testcase.AddTestCases(f, testcases)
	})

	ginkgo.RunSpecs(t, "kubeskoop e2e tests")
}
