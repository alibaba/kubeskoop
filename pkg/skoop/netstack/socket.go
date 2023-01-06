package netstack

import (
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
)

type SockStat int

const (
	SockStatListen = iota
	SockStatEstablish
	SockStatUnknown
)

type ConnStat struct {
	LocalIP    string
	LocalPort  uint16
	RemoteIP   string
	RemotePort uint16
	Protocol   model.Protocol
	State      SockStat
}
