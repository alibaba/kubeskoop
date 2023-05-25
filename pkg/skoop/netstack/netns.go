package netstack

type NetNS struct {
	NetNSInfo    *NetNSInfo
	Interfaces   []Interface
	Router       Router
	IPSetManager *IPSetManager
	IPTables     IPTables
	IPVS         *IPVS
	Netfilter    Netfilter
	Neighbour    *Neighbour
}

// NetNSInfo raw data load from collector
type NetNSInfo struct {
	Netns        string                  `json:"n"`
	NetnsID      string                  `json:"i"`
	PID          uint32                  `json:"p"`
	Key          string                  `json:"k"`
	Interfaces   []Interface             `json:"if"`
	SysctlInfo   map[string]string       `json:"s"`
	RouteInfo    []Route                 `json:"r"`
	RuleInfo     []Rule                  `json:"ru"`
	IptablesInfo string                  `json:"it"`
	IpsetInfo    []*IPSet                `json:"is"`
	IPVSInfo     map[string]*IPVSService `json:"vs"`
	ConnStats    []ConnStat              `json:"c"`
}
