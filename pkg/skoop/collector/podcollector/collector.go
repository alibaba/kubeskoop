package podcollector

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/samber/lo"
	"golang.org/x/sys/unix"

	"google.golang.org/grpc/credentials/insecure"

	"github.com/alibaba/kubeskoop/pkg/skoop/collector"
	"github.com/alibaba/kubeskoop/pkg/skoop/k8s"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/netstack"

	"github.com/bastjan/netstat"
	"github.com/containerd/containerd/pkg/cri/server"
	"github.com/docker/docker/client"
	"github.com/moby/ipvs"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
	pbv1alpha2 "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

type podCollector struct {
	runtimeEndpoint string
	podNamespace    string
	podName         string

	dockerCli          client.APIClient
	runtimeCli         pb.RuntimeServiceClient
	runtimeCliV1Alpha2 pbv1alpha2.RuntimeServiceClient
}

func (a *podCollector) DumpNodeInfos() (*k8s.NodeNetworkStackDump, error) {
	dump := &k8s.NodeNetworkStackDump{}
	var err error
	dump.Pods, err = a.PodList()
	if err != nil {
		return nil, fmt.Errorf("error get pod list, %v", err)
	}

	collectHost := true
	if a.podNamespace == "" && a.podName == "host" {
		// only collect for host namespace
		dump.Pods = nil
	}
	if a.podNamespace != "" && a.podName != "" {
		dump.Pods = lo.Filter(dump.Pods, func(item k8s.PodNetInfo, _ int) bool {
			return item.PodNamespace == a.podNamespace && item.PodName == a.podName
		})
		collectHost = false
	}

	dump.Netns, err = a.SandboxInfos(dump.Pods, collectHost)
	if err != nil {
		return nil, fmt.Errorf("error get sandboxs info, %v", err)
	}

	return dump, nil
}

func NewCollector(podNamespace, podName, runtimeEndpoint string) (collector.Collector, error) {
	pc := &podCollector{podNamespace: podNamespace, podName: podName, runtimeEndpoint: runtimeEndpoint}

	socket := os.Getenv("RUNTIME_SOCK")
	if runtimeEndpoint != "" {
		socket = runtimeEndpoint
	}
	if socket == "" {
		socket = "unix:///var/run/dockershim.sock"
		_, err := os.Stat("/var/run/dockershim.sock")
		if err != nil {
			if os.IsNotExist(err) {
				containerdSockets := []string{
					"unix:///run/containerd/containerd.sock",
					"unix:///run/k3s/containerd/containerd.sock",
				}

				for _, containerdAddr := range containerdSockets {
					if _, err = os.Stat(strings.TrimPrefix(containerdAddr, "unix://")); err == nil {
						socket = containerdAddr
						break
					}
				}
				if socket == "" {
					return nil, fmt.Errorf("cannot found comportable endpoint address for cri-api, please specify cri address by --collector-cri-address")
				}
			} else {
				return nil, err
			}
		} else {
			pc.dockerCli, err = client.NewClientWithOpts(client.WithVersion("1.25"))
			if err != nil {
				return nil, err
			}
		}
	}
	conn, err := grpc.Dial(socket, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	pc.runtimeCli = pb.NewRuntimeServiceClient(conn)
	// negotiate cri api version
	_, err = pc.runtimeCli.Version(context.TODO(), &pb.VersionRequest{})
	if status.Code(err) == codes.Unimplemented {
		pc.runtimeCli = nil
		pc.runtimeCliV1Alpha2 = pbv1alpha2.NewRuntimeServiceClient(conn)
	} else if err != nil {
		return nil, err
	}

	return pc, nil
}

func (a *podCollector) PodInfo(sandbox *pb.PodSandbox) (k8s.PodNetInfo, error) {
	p := k8s.PodNetInfo{}
	var (
		sandboxStatus *pb.PodSandboxStatusResponse
		err           error
	)
	if a.runtimeCli != nil {
		sandboxStatus, err = a.runtimeCli.PodSandboxStatus(context.TODO(), &pb.PodSandboxStatusRequest{
			PodSandboxId: sandbox.Id,
			Verbose:      a.dockerCli == nil,
		})
		if err != nil {
			return p, err
		}
	} else {
		statusAlpha, err := a.runtimeCliV1Alpha2.PodSandboxStatus(context.TODO(), &pbv1alpha2.PodSandboxStatusRequest{
			PodSandboxId: sandbox.Id,
			Verbose:      a.dockerCli == nil,
		})
		if err != nil {
			return p, err
		}
		sandboxStatus = &pb.PodSandboxStatusResponse{}
		if err = alphaRespTov1Resp(statusAlpha, sandboxStatus); err != nil {
			return p, fmt.Errorf("error convert alpha resp %v", err)
		}
	}

	p.PodName = sandboxStatus.Status.GetMetadata().GetName()
	p.PodNamespace = sandboxStatus.Status.GetMetadata().GetNamespace()
	p.ContainerID = sandboxStatus.Status.GetId()
	p.PodUID = sandboxStatus.Status.GetMetadata().Uid

	if a.dockerCli != nil {
		sandboxInfo, err := a.dockerCli.ContainerInspect(context.TODO(), sandbox.Id)
		if err != nil {
			return p, err
		}
		p.PID = uint32(sandboxInfo.State.Pid)
	} else {
		sandboxInfo := server.SandboxInfo{}
		err = json.Unmarshal([]byte(sandboxStatus.GetInfo()["info"]), &sandboxInfo)
		if err != nil {
			return p, err
		}
		p.PID = sandboxInfo.Pid
	}

	if sandboxStatus.Status.GetLinux().GetNamespaces().GetOptions().GetNetwork() == pb.NamespaceMode_POD {
		p.NetworkMode = "none"
		p.Netns = fmt.Sprintf("/proc/%d/ns/net", p.PID)
	} else {
		p.NetworkMode = "host"
		p.Netns = ""
	}
	return p, nil
}

func alphaRespTov1Resp(
	alphaRes interface{ Marshal() ([]byte, error) },
	v1Res interface{ Unmarshal(_ []byte) error },
) error {
	p, err := alphaRes.Marshal()
	if err != nil {
		return err
	}

	return v1Res.Unmarshal(p)
}

func (a *podCollector) PodList() ([]k8s.PodNetInfo, error) {
	var (
		pods     []k8s.PodNetInfo
		sandboxs *pb.ListPodSandboxResponse
		err      error
	)

	if a.runtimeCli != nil {
		sandboxs, err = a.runtimeCli.ListPodSandbox(context.TODO(), &pb.ListPodSandboxRequest{})
		if err != nil {
			return nil, fmt.Errorf("error list pod sandbox: %v", err)
		}
	} else {
		alphaSandboxs, err := a.runtimeCliV1Alpha2.ListPodSandbox(context.TODO(), &pbv1alpha2.ListPodSandboxRequest{})
		if err != nil {
			return nil, fmt.Errorf("error list pod sandbox: %v", err)
		}
		sandboxs = &pb.ListPodSandboxResponse{}
		err = alphaRespTov1Resp(alphaSandboxs, sandboxs)
		if err != nil {
			return nil, fmt.Errorf("error convert alpha pod sandbox: %v", err)
		}
	}

	for _, s := range sandboxs.Items {
		if s.GetState() == pb.PodSandboxState_SANDBOX_READY {
			podinfo, err := a.PodInfo(s)
			if err != nil {
				return nil, err
			}
			pods = append(pods, podinfo)
		}
	}

	return pods, nil
}

func (a *podCollector) SandboxInfos(pods []k8s.PodNetInfo, collectHostNs bool) ([]netstack.NetNSInfo, error) {
	var sandboxInfos []netstack.NetNSInfo
	if collectHostNs {
		hostNsInfo, err := a.SandboxInfo("/proc/1/ns/net", "", 1)
		if err != nil {
			return nil, err
		}
		sandboxInfos = append(sandboxInfos, hostNsInfo)
	}
	for _, p := range pods {
		if p.NetworkMode == "none" {
			nsInfo, err := a.SandboxInfo(p.Netns, fmt.Sprintf("%s/%s", p.PodNamespace, p.PodName), p.PID)
			if err != nil {
				return nil, err
			}
			sandboxInfos = append(sandboxInfos, nsInfo)
		}
	}
	return sandboxInfos, nil
}

func (a *podCollector) SandboxInfo(path, key string, pid uint32) (netstack.NetNSInfo, error) {
	sandboxInfo := netstack.NetNSInfo{
		Netns: path,
		Key:   key,
		PID:   pid,
	}
	var err error
	sandboxInfo.NetnsID, err = getFileInode(path)
	if err != nil {
		return sandboxInfo, err
	}
	for _, infoCollector := range []func(*netstack.NetNSInfo) error{
		interfaceCollector,
		sysctlCollector,
		routeCollector,
		ruleCollector,
		iptablesCollector,
		ipsetCollector,
		ipvsCollector,
		sockCollector,
	} {
		err = nsDo(path, &sandboxInfo, infoCollector)
		if err != nil {
			return sandboxInfo, fmt.Errorf("error run collector %+v, err: %v", runtime.FuncForPC(reflect.ValueOf(infoCollector).Pointer()).Name(), err)
		}
	}
	return sandboxInfo, nil
}

func nsDo(path string, sandboxInfo *netstack.NetNSInfo, f func(sandboxInfo *netstack.NetNSInfo) error) error {
	currentHandler, err := netns.Get()
	if err != nil {
		return err
	}
	defer func() {
		_ = netns.Set(currentHandler)
	}()

	nsHandler, err := netns.GetFromPath(path)
	if err != nil {
		return err
	}
	err = netns.Set(nsHandler)
	if err != nil {
		return err
	}
	return f(sandboxInfo)
}

func getFileInode(path string) (string, error) {
	fileStat, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	fileInfo, ok := fileStat.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("cannot found sysinfo from file stat: %v", path)
	}
	return strconv.FormatUint(fileInfo.Ino, 10), nil
}

// NSExec nsenter {pid} {command}
func NSExec(args []string) (string, error) {
	if len(args) < 3 {
		return "", fmt.Errorf("command args invalid: %v", args)
	}
	fd, err := unix.Open(fmt.Sprintf("/proc/%s/ns/net", args[1]), unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", fmt.Errorf("cannot get pid: %v", err)
	}
	defer func() {
		unix.Close(fd)
	}()

	err = unix.Setns(fd, unix.CLONE_NEWNET)
	if err != nil {
		return "", fmt.Errorf("cannot get pid from pid: %v", err)
	}
	output, err := exec.Command(args[2], args[3:]...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("err:%v, output: %v", err, string(output))
	}
	return string(output), nil
}
func namespaceCmd(pid uint32, cmd string) (string, error) {
	cmdExec := exec.Command("nsenter", strconv.Itoa(int(pid)), "sh", "-c", cmd)
	cmdExec.Path = "/proc/self/exe"
	output, err := cmdExec.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("err:%v, output: %v", err, string(output))
	}
	return string(output), nil
}

func parseSysctls(sysctlsStr string) map[string]string {
	sysctls := map[string]string{}
	for _, sysctlStr := range strings.Split(sysctlsStr, "\n") {
		if sysctlSlice := strings.Split(sysctlStr, "="); len(sysctlSlice) == 2 {
			sysctls[strings.TrimSpace(sysctlSlice[0])] = strings.TrimSpace(sysctlSlice[1])
		}
	}
	return sysctls
}

func interfaceCollector(sandboxInfo *netstack.NetNSInfo) error {
	links, err := netlink.LinkList()
	if err != nil {
		return err
	}
	for _, l := range links {
		attr := netstack.Interface{
			Name:        l.Attrs().Name,
			Index:       l.Attrs().Index,
			MTU:         l.Attrs().MTU,
			Driver:      l.Type(),
			Addrs:       []netstack.Addr{},
			NeighInfo:   []netstack.Neigh{},
			FdbInfo:     []netstack.Neigh{},
			MasterIndex: l.Attrs().MasterIndex,
			PeerIndex:   l.Attrs().ParentIndex,
		}

		switch l.Attrs().OperState {
		case netlink.OperUp:
			attr.State = netstack.LinkUP
		case netlink.OperDown:
			attr.State = netstack.LinkDown
		default:
			if l.Attrs().Flags&net.FlagUp != 0 {
				attr.State = netstack.LinkUP
			} else {
				attr.State = netstack.LinkUnknown
			}
		}

		addrs, err := netlink.AddrList(l, netlink.FAMILY_ALL)
		if err != nil {
			return err
		}
		for _, addr := range addrs {
			attr.Addrs = append(attr.Addrs, netstack.Addr{
				IPNet: addr.IPNet,
			})
		}
		sysctlsStr, err := namespaceCmd(sandboxInfo.PID, fmt.Sprintf("sysctl -a | grep '\\.%s\\.' || true", l.Attrs().Name))
		if err != nil {
			return err
		}
		attr.DevSysctls = parseSysctls(sysctlsStr)
		fdbs, err := netlink.NeighList(l.Attrs().Index, syscall.AF_BRIDGE)
		if err != nil {
			return err
		}
		for _, fdb := range fdbs {
			attr.FdbInfo = append(attr.FdbInfo, netstack.Neigh{
				Family:       syscall.AF_BRIDGE,
				LinkIndex:    l.Attrs().Index,
				State:        fdb.State,
				Type:         fdb.Type,
				Flags:        fdb.Flags,
				IP:           fdb.IP,
				HardwareAddr: fdb.HardwareAddr,
			})
		}

		neighs, err := netlink.NeighList(l.Attrs().Index, netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		for _, neigh := range neighs {
			attr.NeighInfo = append(attr.NeighInfo, netstack.Neigh{
				Family:       netlink.FAMILY_V4,
				LinkIndex:    l.Attrs().Index,
				State:        neigh.State,
				Type:         neigh.Type,
				Flags:        neigh.Flags,
				IP:           neigh.IP,
				HardwareAddr: neigh.HardwareAddr,
			})
		}
		sandboxInfo.Interfaces = append(sandboxInfo.Interfaces, attr)
	}
	return nil
}

func sysctlCollector(sandboxInfo *netstack.NetNSInfo) error {
	sysctlsStr, err := namespaceCmd(sandboxInfo.PID, "sysctl -a || true")
	if err != nil {
		return err
	}
	sandboxInfo.SysctlInfo = parseSysctls(sysctlsStr)
	return nil
}

func routeCollector(sandboxInfo *netstack.NetNSInfo) error {
	rules, err := netlink.RuleList(netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("error collector rule list: %v", err)
	}
	tableIDSet := map[int]interface{}{}
	for _, rule := range rules {
		if _, ok := tableIDSet[rule.Table]; !ok {
			tableIDSet[rule.Table] = struct{}{}
		}
	}

	for tableID := range tableIDSet {
		v4Route, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: tableID}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return fmt.Errorf("error collector route list: %v", err)
		}
		for _, route := range v4Route {
			var iif, oif netlink.Link
			if route.ILinkIndex != 0 {
				iif, err = netlink.LinkByIndex(route.ILinkIndex)
				if err != nil {
					return err
				}
			}
			if route.LinkIndex != 0 {
				oif, err = netlink.LinkByIndex(route.LinkIndex)
				if err != nil {
					return err
				}
			}

			routeInfo := netstack.Route{
				Family:   netlink.FAMILY_V4,
				Scope:    netstack.Scope(route.Scope),
				Dst:      route.Dst,
				Src:      route.Src,
				Gw:       route.Gw,
				Protocol: int(route.Protocol),
				Priority: route.Priority,
				Table:    route.Table,
				Type:     route.Type,
				Tos:      route.Tos,
				Flags:    route.Flags,
			}
			// default route
			if routeInfo.Dst == nil {
				_, routeInfo.Dst, _ = net.ParseCIDR("0.0.0.0/0")
			}

			if iif != nil {
				routeInfo.IifName = iif.Attrs().Name
			}

			if oif != nil {
				routeInfo.OifName = oif.Attrs().Name
			}

			sandboxInfo.RouteInfo = append(sandboxInfo.RouteInfo, routeInfo)
		}
	}
	return nil
}

func ruleCollector(sandboxInfo *netstack.NetNSInfo) error {
	v4Rule, err := netlink.RuleList(netlink.FAMILY_V4)
	if err != nil {
		return err
	}
	sandboxInfo.RuleInfo = []netstack.Rule{}
	for _, rule := range v4Rule {
		sandboxInfo.RuleInfo = append(sandboxInfo.RuleInfo, netstack.Rule{
			Priority: rule.Priority,
			Family:   rule.Family,
			Table:    rule.Table,
			Mark:     rule.Mark,
			Mask:     rule.Mask,
			Tos:      rule.Tos,
			TunID:    rule.TunID,
			Goto:     rule.Goto,
			Src:      rule.Src,
			Dst:      rule.Dst,
			Flow:     rule.Flow,
			IifName:  rule.IifName,
			OifName:  rule.OifName,
		})
	}
	return nil
}

func iptablesCollector(sandboxInfo *netstack.NetNSInfo) error {
	iptableDump, err := namespaceCmd(sandboxInfo.PID, "iptables-save|iptables-xml")
	if err != nil {
		return err
	}
	sandboxInfo.IptablesInfo = iptableDump
	return nil
}

func ipsetCollector(sandboxInfo *netstack.NetNSInfo) error {
	var err error
	info, err := namespaceCmd(sandboxInfo.PID, "ipset list -o xml")
	if err != nil {
		return err
	}
	sandboxInfo.IpsetInfo, err = netstack.ParseIPSet(info)
	return err
}

func ipvsCollector(sandboxInfo *netstack.NetNSInfo) error {
	path := fmt.Sprintf("/proc/%d/ns/net", sandboxInfo.PID)
	handler, err := ipvs.New(path)
	if err != nil {
		return err
	}
	services, err := handler.GetServices()
	if err != nil {
		return err
	}

	m := map[string]*netstack.IPVSService{}
	for _, svc := range services {
		i := &netstack.IPVSService{
			Protocol:  intToProtocol(svc.Protocol),
			IP:        svc.Address.String(),
			Port:      svc.Port,
			Scheduler: svc.SchedName,
			RS:        nil,
		}
		dsts, err := handler.GetDestinations(svc)
		if err != nil {
			return err
		}
		for _, dst := range dsts {
			rs := netstack.RealServer{
				IP:         dst.Address.String(),
				Port:       dst.Port,
				Masquerade: dst.ConnectionFlags == ipvs.ConnectionFlagMasq,
				Weight:     dst.Weight,
			}
			i.RS = append(i.RS, rs)
		}

		m[i.Service()] = i
	}

	sandboxInfo.IPVSInfo = m
	return nil
}

func sockCollector(sandboxInfo *netstack.NetNSInfo) error {
	netstat.ProcRoot = fmt.Sprintf("/proc/%d/", sandboxInfo.PID)
	tcpConns, err := netstat.TCP.Connections()
	if err != nil {
		return fmt.Errorf("error get tcp connections: %v", err)
	}
	tcp6Conns, err := netstat.TCP6.Connections()
	if err != nil {
		return fmt.Errorf("error get tcp6 connections: %v", err)
	}
	tcpConns = append(tcpConns, tcp6Conns...)
	udpConns, err := netstat.UDP.Connections()
	if err != nil {
		return fmt.Errorf("error get udp connections: %v", err)
	}
	udp6Conns, err := netstat.UDP6.Connections()
	if err != nil {
		return fmt.Errorf("error get udp6 connections: %v", err)
	}
	udpConns = append(udpConns, udp6Conns...)
	for _, tc := range tcpConns {
		conn := netstack.ConnStat{
			LocalIP:    tc.IP.String(),
			LocalPort:  uint16(tc.Port),
			RemoteIP:   tc.RemoteIP.String(),
			RemotePort: uint16(tc.RemotePort),
			Protocol:   model.TCP,
		}
		conn.State = netstack.SockStatUnknown
		if tc.State == netstat.TCPEstablished {
			conn.State = netstack.SockStatEstablish
		}
		if tc.State == netstat.TCPListen {
			conn.State = netstack.SockStatListen
		}
		sandboxInfo.ConnStats = append(sandboxInfo.ConnStats, conn)
	}
	for _, tc := range udpConns {
		conn := netstack.ConnStat{
			LocalIP:    tc.IP.String(),
			LocalPort:  uint16(tc.Port),
			RemoteIP:   tc.RemoteIP.String(),
			RemotePort: uint16(tc.RemotePort),
			Protocol:   model.UDP,
		}
		conn.State = netstack.SockStatUnknown
		if tc.State == netstat.TCPEstablished {
			conn.State = netstack.SockStatEstablish
		}
		if slices.Contains([]string{"0.0.0.0", "::"}, tc.RemoteIP.String()) {
			conn.State = netstack.SockStatListen
		}
		sandboxInfo.ConnStats = append(sandboxInfo.ConnStats, conn)
	}
	return nil
}

func intToProtocol(proto uint16) model.Protocol {
	switch proto {
	case unix.IPPROTO_TCP:
		return model.TCP
	case unix.IPPROTO_UDP:
		return model.UDP
	}
	return "unknown"
}
