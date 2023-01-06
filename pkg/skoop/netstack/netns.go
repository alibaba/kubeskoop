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
	Netns        string            `json:"netns"`
	NetnsID      string            `json:"netns_id"`
	PID          uint32            `json:"pid"`
	Key          string            `json:"key"`
	Interfaces   []Interface       `json:"interfaces"`
	SysctlInfo   map[string]string `json:"sysctl_info"`
	RouteInfo    []Route           `json:"route_info"`
	RuleInfo     []Rule            `json:"rule_info"`
	IptablesInfo string            `json:"iptables_info"`
	IpsetInfo    string            `json:"ipset_info"`
	IPVSInfo     []string          `json:"ipvs_info"`
	ConnStats    []ConnStat        `json:"conn_stats"`
}
