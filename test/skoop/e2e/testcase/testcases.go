package testcase

import (
	"github.com/alibaba/kubeskoop/test/skoop/e2e/framework"
	"github.com/alibaba/kubeskoop/test/skoop/e2e/testcase/generic"
	plugin2 "github.com/alibaba/kubeskoop/test/skoop/e2e/testcase/plugin"
)

func AddTestCases(f *framework.Framework, testcases []string) {
	for _, t := range testcases {
		switch t {
		case "generic":
			generic.AddGenericTestCases(f)
		case "generic-service":
			generic.AddGenericServiceTestCases(f)
		case "flannel-hostgw":
			plugin2.AddFlannelHostGwTestCases(f)
		case "flannel-vxlan":
			plugin2.AddFlannelVxlanTestCases(f)
		case "calico":
			plugin2.AddCalicoTestSpecs(f)
		case "terway-veth":
			//todo todo
		case "terway-ipvlan":
			//todo: todo
		}
	}
}
