package main

import (
	"errors"
	"net"
)

var ErrNoIface = errors.New("no suitable IPv4 interface found")

func pickInterface(cfg *Config) (*ifacePick, error) {
	// Force by interface name
	if cfg.MCIface != "" {
		ifi, err := net.InterfaceByName(cfg.MCIface)
		if err != nil {
			return nil, err
		}
		ip, ipn := firstIPv4OnInterface(ifi)
		if ip == nil {
			return nil, errNoIPv4(ifi.Name)
		}
		return &ifacePick{Iface: ifi, IP: ip, IPNet: ipn, IPStr: ip.String(), NetStr: ipn.String(), Forced: true, ByName: true}, nil
	}

	// Pick by subnet
	if cfg.MCSubnet != "" {
		_, target, err := net.ParseCIDR(cfg.MCSubnet)
		if err != nil {
			return nil, err
		}
		ifaces, _ := net.Interfaces()
		for _, ifi := range ifaces {
			addrs, _ := ifi.Addrs()
			for _, a := range addrs {
				ip, ipn, ok := ipv4Net(a)
				if !ok {
					continue
				}
				if target.Contains(ip) {
					return &ifacePick{Iface: &ifi, IP: ip, IPNet: ipn, IPStr: ip.String(), NetStr: ipn.String(), ByCIDR: true}, nil
				}
			}
		}
	}

	// Fallback: first up, non-loopback IPv4
	ifaces, _ := net.Interfaces()
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		ip, ipn := firstIPv4OnInterface(&ifi)
		if ip != nil {
			return &ifacePick{Iface: &ifi, IP: ip, IPNet: ipn, IPStr: ip.String(), NetStr: ipn.String()}, nil
		}
	}
	return nil, ErrNoIface
}

func listMACs() []string {
	var macs []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if m := ifc.HardwareAddr.String(); m != "" {
			macs = append(macs, m)
		}
	}
	return macs
}

func firstIPv4OnInterface(ifi *net.Interface) (net.IP, *net.IPNet) {
	addrs, _ := ifi.Addrs()
	for _, a := range addrs {
		ip, ipn, ok := ipv4Net(a)
		if ok {
			return ip, ipn
		}
	}
	return nil, nil
}

func ipv4Net(a net.Addr) (net.IP, *net.IPNet, bool) {
	switch v := a.(type) {
	case *net.IPNet:
		if ip := v.IP.To4(); ip != nil {
			return ip, v, true
		}
	case *net.IPAddr:
		if ip := v.IP.To4(); ip != nil {
			_, n, _ := net.ParseCIDR(ip.String() + "/32")
			return ip, n, true
		}
	}
	return nil, nil, false
}

// typed error for “iface has no IPv4”
type errNoIPv4 string

func (e errNoIPv4) Error() string { return "interface " + string(e) + " has no IPv4" }
