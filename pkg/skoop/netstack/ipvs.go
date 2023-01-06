package netstack

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"

	"github.com/pkg/errors"
)

type RealServer struct {
	Service    string
	IP         string
	Port       uint16
	Masquerade bool
	Weight     int
}

type IPVSService struct {
	Protocol  model.Protocol
	IP        string
	Port      uint16
	Scheduler string
	RS        []RealServer
}

func (s *IPVSService) Service() string {
	return fmt.Sprintf("%s:%s:%d", s.Protocol, s.IP, s.Port)
}

type IPVS struct {
	services map[string]*IPVSService
}

func (ipvs *IPVS) GetService(proto model.Protocol, ip string, port uint16) *IPVSService {
	key := fmt.Sprintf("%s:%s:%d", strings.ToLower(string(proto)), ip, port)
	return ipvs.services[key]
}

func parseOneLine(ipvs *IPVS, line string) error {
	var (
		addService   bool
		tcpService   string
		udpService   string
		scheduler    string
		addServer    bool
		realServer   string
		masquerading bool
		weight       string
		persistent   string
	)

	ipvsFlags := flag.NewFlagSet("ipvs", flag.ContinueOnError)
	ipvsFlags.BoolVar(&addService, "A", false, "")
	ipvsFlags.StringVar(&tcpService, "t", "", "")
	ipvsFlags.StringVar(&udpService, "u", "", "")
	ipvsFlags.StringVar(&scheduler, "s", "", "")
	ipvsFlags.BoolVar(&addServer, "a", false, "")
	ipvsFlags.StringVar(&realServer, "r", "", "")
	ipvsFlags.BoolVar(&masquerading, "m", false, "")
	ipvsFlags.StringVar(&weight, "w", "", "")
	ipvsFlags.StringVar(&persistent, "p", "", "")
	if err := ipvsFlags.Parse(strings.Fields(line)); err != nil {
		return errors.Wrapf(err, "error parse ipvs rule %s", line)
	}

	if !addServer && !addService {
		return fmt.Errorf("unknown ipvs action")
	}

	var protocol model.Protocol
	var service string
	if udpService != "" {
		protocol = model.UDP
		service = udpService
	} else {
		protocol = model.TCP
		service = tcpService
	}
	arr := strings.Split(service, ":")
	serviceIP := arr[0]
	servicePort, err := strconv.Atoi(arr[1])
	if err != nil {
		return err
	}

	if addService {
		svc := &IPVSService{
			Protocol:  protocol,
			IP:        serviceIP,
			Port:      uint16(servicePort),
			Scheduler: scheduler,
		}
		ipvs.services[svc.Service()] = svc
		return nil
	}

	if addServer {
		weightInt, _ := strconv.Atoi(weight)
		arr := strings.Split(realServer, ":")
		serverIP := arr[0]
		serverPort, err := strconv.Atoi(arr[1])
		if err != nil {
			return err
		}
		backend := RealServer{
			IP:         serverIP,
			Port:       uint16(serverPort),
			Masquerade: masquerading,
			Weight:     weightInt,
		}

		serviceName := fmt.Sprintf("%s:%s", protocol, service)
		svc, ok := ipvs.services[serviceName]
		if !ok {
			return fmt.Errorf("service not exists")
		}

		svc.RS = append(svc.RS, backend)
		return nil
	}

	return nil
}

func ParseIPVS(dump []string) (*IPVS, error) {
	ipvs := &IPVS{
		services: make(map[string]*IPVSService),
	}

	for _, line := range dump {
		if line == "" {
			continue
		}
		if err := parseOneLine(ipvs, line); err != nil {
			return nil, err
		}
	}

	return ipvs, nil
}
