package probe

type Tuple struct {
	Protocol uint8
	Src      string
	Dst      string
	Sport    uint16
	Dport    uint16
}
