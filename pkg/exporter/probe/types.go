package probe

type Tuple struct {
	Protocol uint8
	Sport    uint16
	Dport    uint16
	Src      string
	Dst      string
}
