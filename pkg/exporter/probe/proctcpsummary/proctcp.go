package proctcpsummary

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	log "github.com/sirupsen/logrus"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
)

const (
	ModuleName = "proctcpsummary"

	TCPEstablishedConn = "tcpestablishedconn"
	TCPTimeWaitConn    = "tcptimewaitconn"
	TCPCloseWaitConn   = "tcpclosewaitconn"
	TCPSynSentConn     = "tcpsynsentconn"
	TCPSynRecvConn     = "tcpsynrecvconn"
	TCPTXQueue         = "tcptxqueue"
	TCPRXQueue         = "tcprxqueue"
	TCPListenBacklog   = "tcplistenbacklog"

	// st mapping of tcp state
	/*TCPEstablished:1   TCP_SYN_SENT:2
	TCP_SYN_RECV:3      TCP_FIN_WAIT1:4
	TCP_FIN_WAIT2:5     TCPTimewait:6
	TCP_CLOSE:7         TCP_CLOSE_WAIT:8
	TCP_LAST_ACL:9      TCPListen:10
	TCP_CLOSING:11*/

	TCPEstablished = 1
	TCPSynSent     = 2
	TCPSynRecv     = 3
	TCPTimewait    = 6
	TCPCloseWait   = 8
	TCPListen      = 10

	readLimit = 4294967296 // Byte -> 4 GiB
)

type (
	NetIPSocket []*netIPSocketLine

	NetIPSocketSummary struct {
		TxQueueLength uint64
		RxQueueLength uint64
		UsedSockets   uint64
	}

	netIPSocketLine struct {
		Sl        uint64
		LocalAddr net.IP
		LocalPort uint64
		RemAddr   net.IP
		RemPort   uint64
		St        uint64
		TxQueue   uint64
		RxQueue   uint64
		UID       uint64
		Inode     uint64
	}

	NetTCP []*netIPSocketLine

	NetTCPSummary NetIPSocketSummary
)

var (
	TCPSummaryMetrics = []probe.LegacyMetric{
		{Name: TCPEstablishedConn, Help: "The total number of established TCP connections."},
		{Name: TCPTimeWaitConn, Help: "The total number of TCP connections in the TIME_WAIT state."},
		{Name: TCPCloseWaitConn, Help: "The total number of TCP connections in the CLOSE_WAIT state."},
		{Name: TCPSynSentConn, Help: "The total number of TCP connections in the SYN_SENT state."},
		{Name: TCPSynRecvConn, Help: "The total number of TCP connections in the SYN_RECV state."},
		{Name: TCPTXQueue, Help: "The total size of the TCP transmit queue."},
		{Name: TCPRXQueue, Help: "The total size of the TCP receive queue."},
	}

	probeName = "tcpsummary"
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, softNetProbeCreator)
}

func softNetProbeCreator() (probe.MetricsProbe, error) {
	p := &ProcTCP{}

	batchMetrics := probe.NewLegacyBatchMetrics(probeName, TCPSummaryMetrics, p.CollectOnce)

	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

type ProcTCP struct {
}

func (s *ProcTCP) Start(_ context.Context) error {
	return nil
}

func (s *ProcTCP) Stop(_ context.Context) error {
	return nil
}

func (s *ProcTCP) CollectOnce() (map[string]map[uint32]uint64, error) {
	ets := nettop.GetAllUniqueNetnsEntity()
	if len(ets) == 0 {
		log.Infof("failed collect tcp summary, no entity found")
	}
	return collect(ets), nil
}

func collect(pidlist []*nettop.Entity) map[string]map[uint32]uint64 {
	resMap := make(map[string]map[uint32]uint64)

	for idx := range TCPSummaryMetrics {
		resMap[TCPSummaryMetrics[idx].Name] = map[uint32]uint64{}
	}

	for idx := range pidlist {
		path := fmt.Sprintf("/proc/%d/net/tcp", pidlist[idx].GetPid())
		summary, err := newNetTCP(path)
		if err != nil {
			log.Warnf("failed collect tcp, path %s, err: %v", path, err)
			continue
		}
		summary6, err := newNetTCP(fmt.Sprintf("/proc/%d/net/tcp6", pidlist[idx].GetPid()))
		if err != nil {
			log.Warnf("failed collect tcp6, path %s, err: %v", path, err)
			continue
		}
		est, tw, cw, ss, sr := summary.getEstTwCount()
		est6, tw6, cw6, ss6, sr6 := summary6.getEstTwCount()
		tx, rx := summary.getTxRxQueueLength()
		tx6, rx6 := summary6.getTxRxQueueLength()
		nsinum := uint32(pidlist[idx].GetNetns())
		resMap[TCPEstablishedConn][nsinum] = est + est6
		resMap[TCPTimeWaitConn][nsinum] = tw + tw6
		resMap[TCPCloseWaitConn][nsinum] = cw + cw6
		resMap[TCPSynSentConn][nsinum] = ss + ss6
		resMap[TCPSynRecvConn][nsinum] = sr + sr6
		resMap[TCPTXQueue][nsinum] = tx + tx6
		resMap[TCPRXQueue][nsinum] = rx + rx6
	}

	return resMap
}

func (n NetTCP) getEstTwCount() (est, tw, cw, ss, sr uint64) {
	for idx := range n {
		switch n[idx].St {
		case TCPEstablished:
			est++
		case TCPTimewait:
			tw++
		case TCPCloseWait:
			cw++
		case TCPSynSent:
			ss++
		case TCPSynRecv:
			sr++
		}
	}
	return est, tw, cw, ss, sr
}

func (n NetTCP) getTxRxQueueLength() (tx uint64, rx uint64) {
	for idx := range n {
		if n[idx].St != TCPListen {
			tx += n[idx].TxQueue
			rx += n[idx].RxQueue
		}
	}
	return tx, rx
}

// newNetTCP creates a new NetTCP{,6} from the contents of the given file.
func newNetTCP(file string) (NetTCP, error) {
	n, err := newNetIPSocket(file)
	n1 := NetTCP(n)
	return n1, err
}

func newNetIPSocket(file string) (NetIPSocket, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var netIPSocket NetIPSocket

	lr := io.LimitReader(f, readLimit)
	s := bufio.NewScanner(lr)
	s.Scan() // skip first line with headers
	for s.Scan() {
		fields := strings.Fields(s.Text())
		line, err := parseNetIPSocketLine(fields)
		if err != nil {
			return nil, err
		}
		netIPSocket = append(netIPSocket, line)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return netIPSocket, nil
}

// parseNetIPSocketLine parses a single line, represented by a list of fields.
func parseNetIPSocketLine(fields []string) (*netIPSocketLine, error) {
	line := &netIPSocketLine{}
	if len(fields) < 10 {
		return nil, fmt.Errorf(
			"cannot parse net socket line as it has less then 10 columns %q",
			strings.Join(fields, " "),
		)
	}
	var err error // parse error

	// sl
	s := strings.Split(fields[0], ":")
	if len(s) != 2 {
		return nil, fmt.Errorf("cannot parse sl field in socket line %q", fields[0])
	}

	if line.Sl, err = strconv.ParseUint(s[0], 0, 64); err != nil {
		return nil, fmt.Errorf("cannot parse sl value in socket line: %w", err)
	}
	// local_address
	l := strings.Split(fields[1], ":")
	if len(l) != 2 {
		return nil, fmt.Errorf("cannot parse local_address field in socket line %q", fields[1])
	}
	if line.LocalAddr, err = parseIP(l[0]); err != nil {
		return nil, err
	}
	if line.LocalPort, err = strconv.ParseUint(l[1], 16, 64); err != nil {
		return nil, fmt.Errorf("cannot parse local_address port value in socket line: %w", err)
	}

	// remote_address
	r := strings.Split(fields[2], ":")
	if len(r) != 2 {
		return nil, fmt.Errorf("cannot parse rem_address field in socket line %q", fields[1])
	}
	if line.RemAddr, err = parseIP(r[0]); err != nil {
		return nil, err
	}
	if line.RemPort, err = strconv.ParseUint(r[1], 16, 64); err != nil {
		return nil, fmt.Errorf("cannot parse rem_address port value in socket line: %w", err)
	}

	// st
	if line.St, err = strconv.ParseUint(fields[3], 16, 64); err != nil {
		return nil, fmt.Errorf("cannot parse st value in socket line: %w", err)
	}

	// tx_queue and rx_queue
	q := strings.Split(fields[4], ":")
	if len(q) != 2 {
		return nil, fmt.Errorf(
			"cannot parse tx/rx queues in socket line as it has a missing colon %q",
			fields[4],
		)
	}
	if line.TxQueue, err = strconv.ParseUint(q[0], 16, 64); err != nil {
		return nil, fmt.Errorf("cannot parse tx_queue value in socket line: %w", err)
	}
	if line.RxQueue, err = strconv.ParseUint(q[1], 16, 64); err != nil {
		return nil, fmt.Errorf("cannot parse rx_queue value in socket line: %w", err)
	}

	// uid
	if line.UID, err = strconv.ParseUint(fields[7], 0, 64); err != nil {
		return nil, fmt.Errorf("cannot parse uid value in socket line: %w", err)
	}

	// inode
	if line.Inode, err = strconv.ParseUint(fields[9], 0, 64); err != nil {
		return nil, fmt.Errorf("cannot parse inode value in socket line: %w", err)
	}

	return line, nil
}

func parseIP(hexIP string) (net.IP, error) {
	var byteIP []byte
	byteIP, err := hex.DecodeString(hexIP)
	if err != nil {
		return nil, fmt.Errorf("cannot parse address field in socket line %q", hexIP)
	}
	switch len(byteIP) {
	case 4:
		return net.IP{byteIP[3], byteIP[2], byteIP[1], byteIP[0]}, nil
	case 16:
		i := net.IP{
			byteIP[3], byteIP[2], byteIP[1], byteIP[0],
			byteIP[7], byteIP[6], byteIP[5], byteIP[4],
			byteIP[11], byteIP[10], byteIP[9], byteIP[8],
			byteIP[15], byteIP[14], byteIP[13], byteIP[12],
		}
		return i, nil
	default:
		return nil, fmt.Errorf("unable to parse IP %s", hexIP)
	}
}
