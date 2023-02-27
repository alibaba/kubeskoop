package netstack

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/utils"

	"github.com/beevik/etree"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type XTablesVerdict uint

const (
	XTablesVerdictAccept   XTablesVerdict = 0
	XTablesVerdictDrop     XTablesVerdict = 1
	XTablesVerdictReject   XTablesVerdict = 2
	XTablesVerdictReturn   XTablesVerdict = 3
	XTablesVerdictContinue XTablesVerdict = 4
)

type IPTablesRuleError struct {
	Rule    string
	Message string
}

func (e *IPTablesRuleError) Error() string {
	return fmt.Sprintf("rule: %q, message: %q", e.Rule, e.Message)
}

type ErrIptablesUnsupported struct {
	Message string
}

func (u ErrIptablesUnsupported) Error() string {
	return fmt.Sprintf("unsupported %s", u.Message)
}

type Target interface{}

type ExtensionTarget interface {
	Do(ctx context.Context, packet *model.Packet, iif, oif string) (XTablesVerdict, error)
}

type NopTarget struct{}
type AcceptTarget struct{}
type DropTarget struct{}
type MasqueradeTarget struct{}
type RejectTarget struct{}
type ReturnTarget struct{}
type CallTarget struct {
	Chain string
}
type GotoTarget struct {
	Chain string
}

type DNATTarget struct {
	ToDestination string `ipt:"--to-destination"`
	Random        bool   `ipt:"--random"`
	Persistent    bool   `ipt:"--persistent"`
}

type SNATTarget struct {
	ToSource    string `ipt:"--to-source"`
	Random      bool   `ipt:"--random"`
	RandomFully bool   `ipt:"--random-fully"`
	Persistent  bool   `ipt:"--persistent"`
}

func (s *SNATTarget) Do(ctx context.Context, packet *model.Packet, iif, oif string) (XTablesVerdict, error) {
	return XTablesVerdictAccept, nil
}

func (t *DNATTarget) Do(ctx context.Context, packet *model.Packet, iif, oif string) (XTablesVerdict, error) {
	return XTablesVerdictAccept, nil
}

type MarkTarget struct {
}

func (t *MarkTarget) Do(ctx context.Context, packet *model.Packet, iif, oif string) (XTablesVerdict, error) {
	return XTablesVerdictAccept, nil
}

type NoTrackTarget struct {
}

func (t *NoTrackTarget) Do(ctx context.Context, packet *model.Packet, iif, oif string) (XTablesVerdict, error) {
	return XTablesVerdictAccept, nil
}

type TPProxyTarget struct {
}

func (t *TPProxyTarget) Do(ctx context.Context, packet *model.Packet, iif, oif string) (XTablesVerdict, error) {
	return XTablesVerdictAccept, nil
}

type Matcher interface {
	Match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error)
}

type TCP struct {
	Option string
	Value  uint16
}

func (t *TCP) Match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	if packet.Protocol != model.TCP {
		return false, nil
	}
	switch t.Option {
	case "dport":
		return packet.Dport == t.Value, nil
	case "sport":
		return packet.Sport == t.Value, nil
	default:
		return false, ErrIptablesUnsupported{fmt.Sprintf("tcp match option %s", t.Option)}
	}
}

func (t *TCP) String() string {
	return fmt.Sprintf("--%s %d", t.Option, t.Value)
}

type IP struct {
	Option string
	Value  string
}

func (ip *IP) Match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	switch ip.Option {
	case "i":
		return ip.Value == iif, nil
	case "o":
		return ip.Value == oif, nil
	case "s":
		return utils.IPMatchPrefix(packet.Src, ip.Value)
	case "d":
		return utils.IPMatchPrefix(packet.Dst, ip.Value)
	case "p":
		return strings.EqualFold(string(packet.Protocol), ip.Value), nil
	default:
		return false, ErrIptablesUnsupported{fmt.Sprintf("match option %s", ip.Option)}
	}
}

func (ip *IP) String() string {
	return fmt.Sprintf("-%s %s", ip.Option, ip.Value)
}

type UDP struct {
	Option string
	Value  uint16
}

func (udp *UDP) Match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	if packet.Protocol != model.UDP {
		return false, nil
	}
	switch udp.Option {
	case "dport":
		return packet.Dport == udp.Value, nil
	case "sport":
		return packet.Sport == udp.Value, nil
	default:

		return false, ErrIptablesUnsupported{fmt.Sprintf("udp match option %s", udp.Option)}
	}
}

func (udp *UDP) String() string {
	return fmt.Sprintf("--%s %d", udp.Option, udp.Value)
}

type Conntrack struct {
	Option string
	Value  string
}

func (conntrack *Conntrack) Match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	return true, nil
}
func (conntrack *Conntrack) String() string {
	return fmt.Sprintf("-m conntrack --%s %s", conntrack.Option, conntrack.Value)
}

type Set struct {
	Option string
	Value  string
}

func (set *Set) parseSetArgument() (string, string, error) {
	arr := strings.Fields(set.Value)
	if len(arr) != 2 {
		return "", "", fmt.Errorf("invalid set argument format %s", set.Value)
	}

	return arr[0], arr[1], nil
}

func (set *Set) Match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	setName, flags, err := set.parseSetArgument()
	if err != nil {
		return false, err
	}
	ipsetManager, ok := ctx.Value(ContextIPSetKey).(*IPSetManager)
	if !ok || ipsetManager == nil {
		return false, fmt.Errorf("cannot get ipset from context")
	}

	ipset := ipsetManager.GetIPSet(setName)
	if ipset == nil {
		return false, nil
	}

	ip := func(field string) net.IP {
		switch field {
		case "src":
			return packet.Src
		case "dst":
			return packet.Dst
		default:
			return net.IPv4(0, 0, 0, 0)
		}
	}
	port := func(field string) uint16 {
		switch field {
		case "src":
			return packet.Sport
		case "dst":
			return packet.Dport
		default:
			return 0
		}
	}

	var key string

	switch ipset.Type {
	case "hash:net":
		addr := ip(flags)
		for m := range ipset.Members {
			if match, _ := utils.IPMatchPrefix(addr, m); match {
				return true, nil
			}
		}
		return false, nil
	case "hash:ip,port":
		fields := strings.Split(flags, ",")
		addr := ip(fields[0])
		port := port(fields[1])
		key = fmt.Sprintf("%s,%s:%d", addr, packet.Protocol, port)
	case "hash:ip,port,ip":
		fields := strings.Split(flags, ",")
		addr := ip(fields[0])
		port := port(fields[1])
		addr2 := ip(fields[2])
		key = fmt.Sprintf("%s,%s:%d,%s", addr, packet.Protocol, port, addr2)
	case "bitmap:port":
		port := port(flags)
		key = fmt.Sprintf("%d", port)
	default:
		return false, ErrIptablesUnsupported{fmt.Sprintf("ipset type %s of %s", ipset.Type, setName)}
	}
	_, ok = ipset.Members[key]
	return ok, nil
}

func (set *Set) String() string {
	return fmt.Sprintf("-m set --match-set %s %s", set.Option, set.Value)
}

type Comment struct {
	Option string
	Value  string
}

func (c *Comment) Match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	return true, nil
}

func (c *Comment) String() string {
	return fmt.Sprintf("-m comment --%s %s", c.Option, c.Value)
}

type MultiPort struct {
	Option string
	Value  string
}

func (mp *MultiPort) matchPort(port uint16) (bool, error) {
	fields := strings.Split(mp.Value, ",")
	for _, f := range fields {
		if strings.Contains(f, ":") {
			portRange := strings.Split(f, ":")
			first, _ := strconv.Atoi(portRange[0])
			last, _ := strconv.Atoi(portRange[1])
			if port >= uint16(first) && port <= uint16(last) {
				return true, nil
			}
		} else {
			p, _ := strconv.Atoi(f)
			if port == uint16(p) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (mp *MultiPort) Match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	switch mp.Option {
	case "dports":
		return mp.matchPort(packet.Dport)
	case "sports":
		return mp.matchPort(packet.Sport)
	case "ports":
		ok, err := mp.matchPort(packet.Sport)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
		ok, err = mp.matchPort(packet.Dport)
		if err != nil {
			return false, err
		}
		return ok, nil
	default:
		return false, ErrIptablesUnsupported{fmt.Sprintf("multiport option %s", mp.Option)}
	}
}

func (mp *MultiPort) String() string {
	return fmt.Sprintf("-m multiport --%s %s", mp.Option, mp.Value)
}

func parseMarkValue(s string) (uint32, uint32) {
	if strings.Contains(s, "/") {
		fields := strings.Split(s, "/")
		val, _ := strconv.ParseInt(fields[0], 0, 32)
		mask, _ := strconv.ParseInt(fields[1], 0, 32)
		return uint32(val), uint32(mask)
	}
	val, _ := strconv.ParseInt(s, 0, 32)
	return uint32(val), 0xffffffff
}

type Mark struct {
	Option string
	Value  string
}

func (m *Mark) Match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	mark, mask := parseMarkValue(m.Value)
	return mark == packet.Mark&mask, nil
}

func (m *Mark) String() string {
	return fmt.Sprintf("-m mark --mark %s", m.Value)
}

const (
	RtnUnspec      = 0x0
	RtnUnicast     = 0x1
	RtnLocal       = 0x2
	RtnBroadcast   = 0x3
	RtnAnycast     = 0x4
	RtnMulticast   = 0x5
	RtnBlackhole   = 0x6
	RtnUnreachable = 0x7
	RtnProhibit    = 0x8
	RtnThrow       = 0x9
	RtnNat         = 0xa
	RtnXresolve    = 0xb
)

type AddrType struct {
	Option string
	Value  string
}

func (t *AddrType) Match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	var addr net.IP
	switch t.Option {
	case "src-type":
		addr = packet.Src
	case "dst-type":
		addr = packet.Dst
	case "limit-iface-in":
		return iif == t.Value, nil
	case "limit-iface-out":
		return oif == t.Value, nil
	}

	switch t.Value {
	case "UNSPEC":
		return addr.IsUnspecified(), nil
	case "MULTICAST":
		return addr.IsMulticast(), nil
	}

	router, ok := ctx.Value(ContextRouterKey).(Router)
	if !ok {
		return false, fmt.Errorf("cannot get router from context, router: %#+v", router)
	}

	var addrType int

	route, err := router.TableRoute(RtTableLocal, packet)
	if err != nil {
		if err == ErrNoRouteToHost {
			addrType = RtnUnicast
		} else {
			return false, err
		}
	} else {
		addrType = route.Type
	}

	switch t.Value {
	case "UNICAST":
		return addrType == RtnUnicast, nil
	case "LOCAL":
		return addrType == RtnLocal, nil
	case "BROADCAST":
		return addrType == RtnBroadcast, nil
	case "ANYCAST":
		return addrType == RtnAnycast, nil
	case "MULTICAST":
		return addrType == RtnMulticast, nil
	case "BLACKHOLE":
		return addrType == RtnBlackhole, nil
	case "UNREACHABLE":
		return addrType == RtnUnreachable, nil
	case "PROHIBIT":
		return addrType == RtnProhibit, nil
	}
	return false, nil
}

func (t *AddrType) String() string {
	return fmt.Sprintf("-m addrtype --%s %s", t.Option, t.Value)
}

type Statistic struct {
	Option string
	Value  string
}

func (s *Statistic) Match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	return true, nil
}

func (s *Statistic) String() string {
	return fmt.Sprintf("-m statistic %s %s", s.Option, s.Value)
}

type Physdev struct {
	Option string
	Value  string
}

func (physdev *Physdev) Match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	//FIXME 这里有问题
	return true, nil
}

func (physdev *Physdev) String() string {
	return fmt.Sprintf("-m physdev %s %s", physdev.Option, physdev.Value)
}

type Socket struct {
	Option string
	Value  string
}

func (s *Socket) Socket(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	//FIXME 这里有问题
	return true, nil
}

func (s *Socket) String() string {
	return fmt.Sprintf("-m socket %s %s", s.Option, s.Value)
}

type RPFilter struct {
	Option string
	Value  string
}

func (rp *RPFilter) Match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	if rp.Option == "loose" {
		return true, nil
	}

	router, ok := ctx.Value(ContextRouterKey).(Router)
	if !ok {
		return false, fmt.Errorf("cannot get router from context, router: %#+v", router)
	}
	route, err := router.Route(packet, iif, oif)
	if err != nil {
		if err == ErrNoRouteToHost {
			return false, nil
		}
		return false, err
	}

	match := route.OifName == iif
	if rp.Option == "invert" {
		match = !match
	}
	return match, nil
}

func (rp *RPFilter) String() string {
	return fmt.Sprintf("-m rpfilter %s %s", rp.Option, rp.Value)
}

type match struct {
	matcher Matcher
	invert  bool
}

func (m *match) match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	v, err := m.matcher.Match(ctx, packet, iif, oif)
	if err != nil {
		return v, err
	}

	if m.invert {
		return !v, nil
	}

	return v, nil
}

type rule struct {
	matches []*match
	target  interface{}
}

func (r *rule) match(ctx context.Context, packet *model.Packet, iif, oif string) (bool, error) {
	for _, m := range r.matches {
		v, err := m.match(ctx, packet, iif, oif)
		if err != nil {
			return false, err
		}
		if !v {
			return false, nil
		}
	}
	return true, nil
}

func (r *rule) String() string {
	var matches []string
	for _, m := range r.matches {
		matches = append(matches, fmt.Sprintf("%v", m))
	}
	return fmt.Sprintf("%s -j %v", strings.Join(matches, " "), r.target)
}

type iptChain struct {
	name   string
	policy Verdict
	rules  []*rule
}

type xTable struct {
	name   string
	chains map[string]*iptChain
}

func (xt *xTable) tracePacket(ctx context.Context, hook NFHook, packet *model.Packet, iif, oif string) (verdict Verdict, trace Trace, err error) {
	type stackFrame struct {
		chain *iptChain
		pos   int
	}

	chain := xt.chains[hook.String()]
	stack := utils.NewStack[*stackFrame](&stackFrame{
		chain: chain,
		pos:   0,
	})

	verdict = VerdictDrop

	var frame *stackFrame

	buildTrace := func(frame *stackFrame) {
		if frame == nil {
			return
		}

		if frame.pos >= len(frame.chain.rules) {
			return
		}

		trace = append(trace, fmt.Sprintf("%s %s %s", xt.name, chain.name, frame.chain.rules[frame.pos]))
	}

	defer func() {
		if verdict != VerdictAccept {
			buildTrace(frame)
			for !stack.Empty() {
				buildTrace(stack.Pop())
			}
		}
	}()

	for !stack.Empty() {
		frame = stack.Pop()
	chain:
		for pos := frame.pos; pos < len(frame.chain.rules); {
			rule := frame.chain.rules[pos]
			pos++

			v, err := rule.match(ctx, packet, iif, oif)
			if err != nil {
				return VerdictDrop, trace, &IPTablesRuleError{Rule: rule.String(), Message: err.Error()}
			}

			if v {
				trace = append(trace, fmt.Sprintf("%s %s %s", xt.name, chain.name, rule))
				switch target := rule.target.(type) {
				case *AcceptTarget:
					return VerdictAccept, trace, nil
				case *DropTarget, RejectTarget:
					return VerdictDrop, trace, nil
				case *NopTarget:
					continue
				case *ReturnTarget:
					break chain
				case *CallTarget:
					stack.Push(&stackFrame{chain, pos})
					targetChain := xt.chains[target.Chain]
					stack.Push(&stackFrame{targetChain, 0})
					break chain
				case *GotoTarget:
					targetChain := xt.chains[target.Chain]
					stack.Push(&stackFrame{targetChain, 0})
					break chain
				case ExtensionTarget:
					verdict, err := target.Do(ctx, packet, iif, oif)
					if err != nil {
						return VerdictDrop, trace, err
					}
					switch verdict {
					case XTablesVerdictAccept:
						return VerdictAccept, trace, nil
					case XTablesVerdictDrop, XTablesVerdictReject:
						return VerdictDrop, trace, nil
					case XTablesVerdictContinue:
						continue
					case XTablesVerdictReturn:
						break chain
					default:
						panic(fmt.Sprintf("unknown verdict %v", verdict))
					}
				default:
					return VerdictDrop, trace, &IPTablesRuleError{Rule: rule.String(), Message: fmt.Sprintf("unsupported target (%T)%v", target, target)}
				}
			}
		}
	}
	return chain.policy, trace, nil
}

type IPTables interface {
	TracePacket(ctx context.Context, hook NFHook, table string, packet *model.Packet, iif, oif string) (Verdict, Trace, error)
	Empty() error
	DefaultAccept() error
}

func createEmptyTable(name string, chains ...string) *xTable {
	iptChains := make(map[string]*iptChain)
	for _, n := range chains {
		iptChains[n] = &iptChain{name: n, policy: VerdictAccept}
	}

	return &xTable{
		name:   name,
		chains: iptChains,
	}
}

type emptyIPTables struct {
	xTables map[string]*xTable
}

func (ipt *emptyIPTables) TracePacket(ctx context.Context, hook NFHook, table string, packet *model.Packet, iif, oif string) (Verdict, Trace, error) {
	return VerdictAccept, nil, nil
}

func (ipt *emptyIPTables) Empty() error {
	return nil
}

func (ipt *emptyIPTables) DefaultAccept() error {
	return nil
}

func createEmptyIPTables() *emptyIPTables {
	return &emptyIPTables{
		xTables: map[string]*xTable{
			"raw":    createEmptyTable("raw", "PREROUTING", "OUTPUT"),
			"mangle": createEmptyTable("mangle", "PREROUTING", "INPUT", "FORWARD", "OUTPUT", "POSTROUTING"),
			"nat":    createEmptyTable("nat", "PREROUTING", "INPUT", "OUTPUT", "POSTROUTING"),
			"filter": createEmptyTable("filter", "INPUT", "FORWARD", "OUTPUT"),
		},
	}
}

type defaultIPTables struct {
	xTables map[string]*xTable
}

type Trace []string

func (t Trace) String() string {
	var indent []string
	for _, s := range t {
		indent = append(indent, fmt.Sprintf("    %s", s))
	}
	return strings.Join(indent, "\n")
}

func (ipt *defaultIPTables) TracePacket(ctx context.Context, hook NFHook, table string, packet *model.Packet, iif, oif string) (Verdict, Trace, error) {
	var trace Trace
	xtable := ipt.xTables[table]
	if xtable == nil {
		return VerdictAccept, trace, nil
	}
	verdict, trace, err := xtable.tracePacket(ctx, hook, packet, iif, oif)
	return verdict, trace, err
}

func (ipt *defaultIPTables) Empty() error {
	for _, tbl := range ipt.xTables {
		for _, chain := range tbl.chains {
			if len(chain.rules) > 0 {
				return fmt.Errorf("table %s chain %s is not empty", tbl.name, chain.name)
			}
		}
	}
	return nil
}

func (ipt *defaultIPTables) DefaultAccept() error {
	for _, tbl := range ipt.xTables {
		for _, chain := range tbl.chains {
			if chain.policy != VerdictAccept {
				return fmt.Errorf("table %s chain %s default policy is not ACCEPT", tbl.name, chain.name)
			}
		}
	}
	return nil
}

var ModuleTypes = map[string]reflect.Type{
	"tcp":       reflect.TypeOf(TCP{}),
	"udp":       reflect.TypeOf(UDP{}),
	"match":     reflect.TypeOf(IP{}),
	"set":       reflect.TypeOf(Set{}),
	"comment":   reflect.TypeOf(Comment{}),
	"multiport": reflect.TypeOf(MultiPort{}),
	"mark":      reflect.TypeOf(Mark{}),
	"statistic": reflect.TypeOf(Statistic{}),
	"conntrack": reflect.TypeOf(Conntrack{}),
	"addrtype":  reflect.TypeOf(AddrType{}),
	"rpfilter":  reflect.TypeOf(RPFilter{}),
}

var ActionTypes = map[string]reflect.Type{
	"DNAT":       reflect.TypeOf(DNATTarget{}),
	"SNAT":       reflect.TypeOf(SNATTarget{}),
	"MASQUERADE": reflect.TypeOf(MasqueradeTarget{}),
	"MARK":       reflect.TypeOf(MarkTarget{}),
	"ACCEPT":     reflect.TypeOf(AcceptTarget{}),
	"DROP":       reflect.TypeOf(DropTarget{}),
	"RETURN":     reflect.TypeOf(ReturnTarget{}),
	"REJECT":     reflect.TypeOf(RejectTarget{}),
	"NOTRACK":    reflect.TypeOf(NoTrackTarget{}),
	"TPROXY":     reflect.TypeOf(TPProxyTarget{}),
}

func setField(s reflect.Value, fieldName string, value string) error {
	t := s.Type()
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("%s is not a struct", s)
	}
	_, ok := t.FieldByName(fieldName)
	if !ok {
		return fmt.Errorf("%s has no field %s", s, fieldName)
	}

	field := s.FieldByName(fieldName)

	switch field.Type().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("error parse %s to int", value))
		}
		field.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("error parse %s to uint", value))
		}
		field.SetUint(i)
	case reflect.Float32:
		f, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("error parse %s to float32", value))
		}
		field.SetFloat(f)
	case reflect.Float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("error parse %s to float64", value))
		}
		field.SetFloat(f)
	case reflect.String:
		field.SetString(value)
	default:
		return fmt.Errorf("unsupported field type %s", field.Type().Kind().String())
	}
	return nil
}

func createMatcher(moduleType reflect.Type, matchField string, value string) (Matcher, error) {
	module := reflect.New(moduleType)
	if err := setField(module.Elem(), "Option", matchField); err != nil {
		return nil, err
	}
	if err := setField(module.Elem(), "Value", value); err != nil {
		return nil, err
	}
	return module.Interface().(Matcher), nil
}

func createTarget(actionType reflect.Type, params map[string]string) (Target, error) {
	action := reflect.New(actionType)
	for i := 0; i < actionType.NumField(); i++ {
		field := actionType.Field(i)
		tag, ok := field.Tag.Lookup("ipt")
		if !ok {
			continue
		}
		val, ok := params[tag]
		if !ok {
			continue
		}
		if err := setField(action.Elem(), field.Name, val); err != nil {
			return nil, err
		}
	}
	return action.Interface(), nil
}

func parseOneRule(xmlRule *etree.Element) (*rule, error) {
	var conditions []*match
	xmlConditions := xmlRule.FindElement("conditions")
	if xmlConditions != nil {
		for _, xmlModule := range xmlConditions.ChildElements() {
			moduleKey := xmlModule.Tag
			moduleType, ok := ModuleTypes[moduleKey]
			if !ok {
				return nil, fmt.Errorf("unspported match module %s", moduleKey)
			}
			for _, xmlExpr := range xmlModule.ChildElements() {
				key := xmlExpr.Tag
				value := xmlExpr.Text()
				invertAttr := xmlExpr.SelectAttr("invert")
				invert := invertAttr != nil && invertAttr.Value == "true"
				matcher, err := createMatcher(moduleType, key, value)
				if err != nil {
					panic(err)
				}
				conditions = append(conditions, &match{matcher: matcher, invert: invert})
			}
		}
	}

	var target Target
	xmlAction := xmlRule.FindElement("actions")
	if xmlAction == nil || len(xmlAction.ChildElements()) == 0 {
		target = &NopTarget{}
		return &rule{matches: conditions, target: target}, nil
	}

	action := xmlAction.ChildElements()[0]

	if action.Tag == "call" {
		target = &CallTarget{Chain: action.ChildElements()[0].Tag}
	} else if action.Tag == "goto" {
		target = &CallTarget{Chain: action.ChildElements()[0].Tag}
	} else {
		actionType, ok := ActionTypes[action.Tag]
		if !ok {
			return nil, fmt.Errorf("unsupported action %s", action.Tag)
		}
		params := make(map[string]string)
		for _, child := range action.ChildElements() {
			params[child.Tag] = child.Text()
		}
		var err error
		target, err = createTarget(actionType, params)
		if err != nil {
			return nil, errors.Wrapf(err, "error create target %s, err: %v", action.Tag, err)
		}
	}

	return &rule{matches: conditions, target: target}, nil
}
func parseOneTable(xmlTable *etree.Element) (*xTable, error) {
	tableName := xmlTable.SelectAttr("name").Value
	chains := make(map[string]*iptChain)
	for _, xmlChain := range xmlTable.ChildElements() {
		policy := VerdictAccept
		name := xmlChain.SelectAttr("name").Value
		policyAttr := xmlChain.SelectAttr("policy")
		if policyAttr != nil && policyAttr.Value == "DROP" {
			policy = VerdictDrop
		}
		var rules []*rule
		for _, xmlRule := range xmlChain.ChildElements() {
			rule, err := parseOneRule(xmlRule)
			if err != nil {
				return nil, err
			}
			rules = append(rules, rule)
		}
		chains[name] = &iptChain{
			name:   name,
			policy: policy,
			rules:  rules,
		}

	}
	return &xTable{name: tableName, chains: chains}, nil
}

func ParseIPTables(dump string) (ipt IPTables) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error parse iptables, err: %v", r)
			ipt = createEmptyIPTables()
		}
	}()
	if dump == "" {
		return createEmptyIPTables()
	}
	doc := etree.NewDocument()
	err := doc.ReadFromString(dump)
	if err != nil {
		panic(err)
	}

	tables := make(map[string]*xTable)
	for _, xmlTable := range doc.Root().ChildElements() {
		table, err := parseOneTable(xmlTable)
		if err != nil {
			panic(err)
		}
		tables[table.name] = table
	}
	return &defaultIPTables{xTables: tables}
}
