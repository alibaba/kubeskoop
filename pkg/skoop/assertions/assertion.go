package assertions

import (
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
)

type Assertion interface {
	AddSuspicion(level model.SuspicionLevel, msg string)
}

func AssertTrue(assertion Assertion, test bool, level model.SuspicionLevel, msg string) {
	if !test {
		assertion.AddSuspicion(level, msg)
	}
}

func AssertNotTrue(assertion Assertion, test bool, level model.SuspicionLevel, msg string) {
	AssertTrue(assertion, !test, level, msg)
}
