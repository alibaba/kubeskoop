package assertions

import (
	"testing"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/netstack"

	"github.com/stretchr/testify/assert"
)

type suspicionList struct {
	Suspicions []model.Suspicion
}

func (s *suspicionList) AddSuspicion(level model.SuspicionLevel, message string) {
	s.Suspicions = append(s.Suspicions, model.Suspicion{
		Level:   level,
		Message: message,
	})
}

func TestAssertNetDevice(t *testing.T) {
	list := &suspicionList{}
	netAss := NewNetstackAssertion(list, &netstack.NetNS{Interfaces: []netstack.Interface{
		{
			Name:   "eth0",
			MTU:    1450,
			Driver: "veth",
		},
	}})

	netAss.AssertNetDevice("eth0", netstack.Interface{
		Driver: "ipvlan",
		MTU:    1450,
	})
	assert.Equal(t, 1, len(list.Suspicions))
}
