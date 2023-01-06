package assertions

import (
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
)

type CannotBuildTransmissionError struct {
	SrcNode *model.NetNode
	Err     error
}

func (e *CannotBuildTransmissionError) Error() string {
	return e.Err.Error()
}
