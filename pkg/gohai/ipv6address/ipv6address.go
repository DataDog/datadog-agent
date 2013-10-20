package ipv6address

import (
	"errors"
	"net"
)

type Ipv6Address struct{}

const name = "ipv6address"

func (self *Ipv6Address) Name() string {
	return name
}

func (self *Ipv6Address) Collect() (result interface{}, err error) {
	ipv6address, err := externalIpv6Address()
	result = ipv6address

	return
}

func externalIpv6Address() (string, error) {
	ifaces, err := net.Interfaces()

	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			// interface down or loopback interface
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip.To4() != nil {
				// ipv4 address
				continue
			}
			return ip.String(), nil
		}
	}
	return "", errors.New("not connected to the network")
}
