package model

import "fmt"

type Protocol string

const (
	TCP  Protocol = "tcp"
	UDP  Protocol = "udp"
	IPv4 Protocol = "ipv4"
)

const (
	SuspicionLevelInfo = iota
	SuspicionLevelWarning
	SuspicionLevelCritical
	SuspicionLevelFatal
)

type EndpointType string

const (
	EndpointTypePod          = "pod"
	EndpointTypeNode         = "node"
	EndpointTypeService      = "service"
	EndpointTypeLoadbalancer = "lb"
	EndpointTypeExternal     = "external"
)

type Endpoint struct {
	IP   string       `json:"ip"`
	Type EndpointType `json:"type"`
	Port uint16       `json:"port"`
}

func (e Endpoint) String() string {
	return fmt.Sprintf("[%s]%s:%d", e.Type, e.IP, e.Port)
}

type SuspicionLevel int

type Hop struct {
	Type NetNodeType
	ID   string
}

type Transmission struct {
	NextHop Hop
	Link    *Link
}

func (s SuspicionLevel) String() string {
	switch int(s) {
	case SuspicionLevelInfo:
		return "INFO"
	case SuspicionLevelWarning:
		return "WARNING"
	case SuspicionLevelCritical:
		return "CRITICAL"
	case SuspicionLevelFatal:
		return "FATAL"
	default:
		return ""
	}
}

type Suspicion struct {
	Level   SuspicionLevel
	Message string
}
