package testcase

import (
	"github.com/alibaba/kubeskoop/test/skoop/e2e/framework"
	"github.com/alibaba/kubeskoop/test/skoop/e2e/testcase/generic"
	"github.com/alibaba/kubeskoop/test/skoop/e2e/testcase/plugin"
)

func AddTestCases(f *framework.Framework, testcases []string) {
	for _, t := range testcases {
		switch t {
		case "generic":
			generic.AddGenericTestCases(f)
		case "generic-service":
			generic.AddGenericServiceTestCases(f)
		case "flannel-hostgw":
			plugin.AddFlannelHostGwTestCases(f)
		case "flannel-vxlan":
			plugin.AddFlannelVxlanTestCases(f)
		case "calico":
			plugin.AddCalicoTestSpecs(f)
		case "terway-veth":
			//todo todo
		case "terway-ipvlan":
			//todo: todo
		}
	}
}
