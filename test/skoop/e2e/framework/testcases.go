package framework

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func GenerateTestCases(f *Framework, testcases []*TestSpec) {

	AfterEach(func() {
		By("Recover")
		err := f.Recover()
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	})

	for _, t := range testcases {
		t := t
		It(t.Name, func() {
			f.SetTestSpec(t)

			By("Assign nodes")
			err := f.AssignNodes()
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			By("Prepare")
			err = f.Prepare()
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			By("Run diagnose")
			err = f.RunDiagnose()
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			By("Assert")
			err = f.Assert()
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})
	}
}
