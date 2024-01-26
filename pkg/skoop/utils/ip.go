package utils

import (
	"fmt"
	"net"
	"strings"
)

func MatchPrefix(ip, cidr string) (bool, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, err
	}
	ipobj := net.ParseIP(ip)
	if ipobj == nil {
		return false, fmt.Errorf("error parse ip object: %v", ip)
	}
	return ipnet.Contains(ipobj), nil
}

func IPMatchPrefix(ip net.IP, cidr string) (bool, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, err
	}
	return ipnet.Contains(ip), nil
}

func CompareRoute() {
	panic("xx")
}

func ConvertNICNameInSysctls(nicName string) string {
	return strings.Replace(nicName, ".", "/", -1)
}
