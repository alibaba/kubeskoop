// nolint
package plugin

import (
	. "github.com/alibaba/kubeskoop/test/skoop/e2e/framework"
	"github.com/onsi/ginkgo/v2"
)

var terwayVethTestSpecs = []*TestSpec{}

func AddTerwayVEthTestSpecs(f *Framework) {
	ginkgo.Describe("terway veth", func() {
		GenerateTestCases(f, terwayVethTestSpecs)
	})
}
