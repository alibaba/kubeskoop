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
	LocalIP    string         `json:"l"`
	LocalPort  uint16         `json:"lp"`
	RemoteIP   string         `json:"r"`
	RemotePort uint16         `json:"rp"`
	Protocol   model.Protocol `json:"p"`
	State      SockStat       `json:"st"`
}
