package procsock

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"

	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"

	"github.com/prometheus/procfs"
)

const (
	TCPSockInuse    = "Inuse"
	TCPSockOrphan   = "Orphan"
	TCPSockTimewait = "TW"
	TCPSockeAlloc   = "Alloc"
	TCPSockeMem     = "Mem"

	MODULE_NAME = "procsock"
)

var (
	TCPSockStatMetrics = []string{TCPSockInuse, TCPSockOrphan, TCPSockTimewait, TCPSockeAlloc, TCPSockeMem}
	probe              = &ProcSock{}
)

func GetProbe() *ProcSock {
	return probe
}

func (s *ProcSock) Close() error {
	return nil
}

func (s *ProcSock) Start(_ context.Context) {
}

func (s *ProcSock) Ready() bool {
	// determine by if default snmp file was ready
	if _, err := os.Stat("/proc/net/snmp"); os.IsNotExist(err) {
		return false
	}
	return true
}

func (s *ProcSock) Name() string {
	return MODULE_NAME
}

func (s *ProcSock) GetMetricNames() []string {
	res := []string{}
	for _, m := range TCPSockStatMetrics {
		res = append(res, metricUniqueId("sock", m))
	}
	return res
}

func (s *ProcSock) Collect(ctx context.Context) (map[string]map[uint32]uint64, error) {
	return collect(ctx)
}

func metricUniqueId(subject string, m string) string {
	return fmt.Sprintf("%s%s", subject, strings.ToLower(m))
}

type ProcSock struct {
}

type tcpsockstat struct {
	InUse  int
	Orphan int
	TW     int
	Alloc  int
	Mem    int
}

func collect(ctx context.Context) (resMap map[string]map[uint32]uint64, err error) {
	resMap = make(map[string]map[uint32]uint64)
	for _, stat := range TCPSockStatMetrics {
		resMap[metricUniqueId("sock", stat)] = map[uint32]uint64{}
	}

	// for _, nslogic := range nslist {
	// 	skstat, err := getTcpSockstatByPid(uint32(nslogic.GetPid()))
	// 	if err != nil {
	// 		continue
	// 	}
	// 	nsinum := uint32(nslogic.GetNetns())
	// 	resMap[metricUniqueId("sock", TCPSockInuse)][nsinum] = uint64(skstat.InUse)
	// 	resMap[metricUniqueId("sock", TCPSockOrphan)][nsinum] = uint64(skstat.Orphan)
	// 	resMap[metricUniqueId("sock", TCPSockTimewait)][nsinum] = uint64(skstat.TW)
	// 	resMap[metricUniqueId("sock", TCPSockeAlloc)][nsinum] = uint64(skstat.Alloc)
	// 	resMap[metricUniqueId("sock", TCPSockeMem)][nsinum] = uint64(skstat.Mem)
	// }
	skstat, err := getHostTcpSockstat()
	if err != nil {
		return resMap, err
	}
	nsinum := uint32(nettop.InitNetns)
	resMap[metricUniqueId("sock", TCPSockInuse)][nsinum] = uint64(skstat.InUse)
	resMap[metricUniqueId("sock", TCPSockOrphan)][nsinum] = uint64(skstat.Orphan)
	resMap[metricUniqueId("sock", TCPSockTimewait)][nsinum] = uint64(skstat.TW)
	resMap[metricUniqueId("sock", TCPSockeAlloc)][nsinum] = uint64(skstat.Alloc)
	resMap[metricUniqueId("sock", TCPSockeMem)][nsinum] = uint64(skstat.Mem)

	return
}

// getProcessTcpSockstat only fetch a process in an netns, just want tcp info
// func getTcpSockstatByPid(pid uint32) (tcpsockstat, error) {
// 	res := tcpsockstat{}
// 	data, err := ReadFileNoStat(fmt.Sprintf("/proc/%d/net/sockstat", pid))
// 	if err != nil {
// 		return res, err
// 	}

// 	stat, err := parseSockstat(bytes.NewReader(data))
// 	if err != nil {
// 		return res, err
// 	}

// 	for idx := range stat.Protocols {
// 		if strings.Compare(stat.Protocols[idx].Protocol, "TCP") == 0 {
// 			res.InUse = stat.Protocols[idx].InUse
// 			res.Orphan = *stat.Protocols[idx].Orphan
// 			res.Alloc = *stat.Protocols[idx].Alloc
// 			res.TW = *stat.Protocols[idx].TW
// 			res.Mem = *stat.Protocols[idx].Mem
// 		}
// 	}
// 	data6, err := ReadFileNoStat(fmt.Sprintf("/proc/%d/net/sockstat6", pid))
// 	if err != nil {
// 		// if ipv6 stat not available, use ipv4 data directly
// 		return res, nil
// 	}

// 	stat6, err := parseSockstat(bytes.NewReader(data6))
// 	if err != nil {
// 		return res, nil
// 	}
// 	for idx := range stat6.Protocols {
// 		if strings.Compare(stat.Protocols[idx].Protocol, "TCP") == 0 {
// 			res.InUse += stat.Protocols[idx].InUse
// 			res.Orphan += *stat.Protocols[idx].Orphan
// 			res.Alloc += *stat.Protocols[idx].Alloc
// 			res.TW += *stat.Protocols[idx].TW
// 			res.Mem += *stat.Protocols[idx].Mem
// 		}
// 	}

// 	return res, nil
// }

func getHostTcpSockstat() (tcpsockstat, error) {
	res := tcpsockstat{}
	data, err := ReadFileNoStat("/proc/net/sockstat")
	if err != nil {
		return res, err
	}

	stat, err := parseSockstat(bytes.NewReader(data))
	if err != nil {
		return res, err
	}

	for idx := range stat.Protocols {
		if strings.Compare(stat.Protocols[idx].Protocol, "TCP") == 0 {
			res.InUse = stat.Protocols[idx].InUse
			res.Orphan = *stat.Protocols[idx].Orphan
			res.Alloc = *stat.Protocols[idx].Alloc
			res.TW = *stat.Protocols[idx].TW
			res.Mem = *stat.Protocols[idx].Mem
		}
	}
	data6, err := ReadFileNoStat("/proc/net/sockstat6")
	if err != nil {
		// if ipv6 stat not available, use ipv4 data directly
		return res, nil
	}

	stat6, err := parseSockstat(bytes.NewReader(data6))
	if err != nil {
		return res, nil
	}
	for idx := range stat6.Protocols {
		if strings.Compare(stat.Protocols[idx].Protocol, "TCP") == 0 {
			res.InUse += stat.Protocols[idx].InUse
			res.Orphan += *stat.Protocols[idx].Orphan
			res.Alloc += *stat.Protocols[idx].Alloc
			res.TW += *stat.Protocols[idx].TW
			res.Mem += *stat.Protocols[idx].Mem
		}
	}

	return res, nil
}

// parseSockstat reads the contents of a sockstat file and parses a NetSockstat.
func parseSockstat(r io.Reader) (*procfs.NetSockstat, error) {
	var stat procfs.NetSockstat
	s := bufio.NewScanner(r)
	for s.Scan() {
		// Expect a minimum of a protocol and one key/value pair.
		fields := strings.Split(s.Text(), " ")
		if len(fields) < 3 {
			return nil, fmt.Errorf("malformed sockstat line: %q", s.Text())
		}

		// The remaining fields are key/value pairs.
		kvs, err := parseSockstatKVs(fields[1:])
		if err != nil {
			return nil, fmt.Errorf("error parsing sockstat key/value pairs from %q: %w", s.Text(), err)
		}

		// The first field is the protocol. We must trim its colon suffix.
		proto := strings.TrimSuffix(fields[0], ":")
		switch proto {
		case "sockets":
			// Special case: IPv4 has a sockets "used" key/value pair that we
			// embed at the top level of the structure.
			used := kvs["used"]
			stat.Used = &used
		default:
			// Parse all other lines as individual protocols.
			nsp := parseSockstatProtocol(kvs)
			nsp.Protocol = proto
			stat.Protocols = append(stat.Protocols, nsp)
		}
	}

	if err := s.Err(); err != nil {
		return nil, err
	}

	return &stat, nil
}

// parseSockstatKVs parses a string slice into a map of key/value pairs.
func parseSockstatKVs(kvs []string) (map[string]int, error) {
	if len(kvs)%2 != 0 {
		return nil, errors.New("odd number of fields in key/value pairs")
	}

	// Iterate two values at a time to gather key/value pairs.
	out := make(map[string]int, len(kvs)/2)
	for i := 0; i < len(kvs); i += 2 {
		vp := NewValueParser(kvs[i+1])
		out[kvs[i]] = vp.Int()

		if err := vp.Err(); err != nil {
			return nil, err
		}
	}

	return out, nil
}

// parseSockstatProtocol parses a NetSockstatProtocol from the input kvs map.
func parseSockstatProtocol(kvs map[string]int) procfs.NetSockstatProtocol {
	var nsp procfs.NetSockstatProtocol
	for k, v := range kvs {
		// Capture the range variable to ensure we get unique pointers for
		// each of the optional fields.
		v := v
		switch k {
		case "inuse":
			nsp.InUse = v
		case "orphan":
			nsp.Orphan = &v
		case "tw":
			nsp.TW = &v
		case "alloc":
			nsp.Alloc = &v
		case "mem":
			nsp.Mem = &v
		case "memory":
			nsp.Memory = &v
		}
	}

	return nsp
}

type ValueParser struct {
	v   string
	err error
}

// NewValueParser creates a ValueParser using the input string.
func NewValueParser(v string) *ValueParser {
	return &ValueParser{v: v}
}

// Int interprets the underlying value as an int and returns that value.
func (vp *ValueParser) Int() int { return int(vp.int64()) }

// PInt64 interprets the underlying value as an int64 and returns a pointer to
// that value.
func (vp *ValueParser) PInt64() *int64 {
	if vp.err != nil {
		return nil
	}

	v := vp.int64()
	return &v
}

// int64 interprets the underlying value as an int64 and returns that value.
// TODO: export if/when necessary.
func (vp *ValueParser) int64() int64 {
	if vp.err != nil {
		return 0
	}

	// A base value of zero makes ParseInt infer the correct base using the
	// string's prefix, if any.
	const base = 0
	v, err := strconv.ParseInt(vp.v, base, 64)
	if err != nil {
		vp.err = err
		return 0
	}

	return v
}

// PUInt64 interprets the underlying value as an uint64 and returns a pointer to
// that value.
func (vp *ValueParser) PUInt64() *uint64 {
	if vp.err != nil {
		return nil
	}

	// A base value of zero makes ParseInt infer the correct base using the
	// string's prefix, if any.
	const base = 0
	v, err := strconv.ParseUint(vp.v, base, 64)
	if err != nil {
		vp.err = err
		return nil
	}

	return &v
}

// Err returns the last error, if any, encountered by the ValueParser.
func (vp *ValueParser) Err() error {
	return vp.err
}

func ReadFileNoStat(filename string) ([]byte, error) {
	const maxBufferSize = 1024 * 512

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := io.LimitReader(f, maxBufferSize)
	return io.ReadAll(reader)
}
