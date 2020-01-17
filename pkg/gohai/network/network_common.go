package network

import (
	"errors"
	"net"
)

type Network struct{}

const name = "network"

func (self *Network) Name() string {
	return name
}

func (self *Network) Collect() (result interface{}, err error) {
	result, err = getNetworkInfo()
	interfaces := getMultiNetworkInfo()
	if len(interfaces) > 0 {
		interfaceMap, ok := result.(map[string]interface{})
		if !ok {
			return
		}
		interfaceMap["interfaces"] = interfaces
	}
	return
}

func getMultiNetworkInfo() (multiNetworkInfo []map[string]interface{}) {
	ifaces, err := net.Interfaces()

	if err != nil {
		return nil
	}
	for _, iface := range ifaces {
		_iface := make(map[string]interface{})
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			// interface down or loopback interface
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil
		}
		for _, addr := range addrs {
			ip, network, _ := net.ParseCIDR(addr.String())
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip.To4() == nil {
				_iface["ipv6"] = ip.String()
				_iface["ipv6-network"] = network.String()
			} else {
				_iface["ipv4"] = ip.String()
				_iface["ipv4-network"] = network.String()
			}
			if len(iface.HardwareAddr.String()) > 0 {
				_iface["macaddress"] = iface.HardwareAddr.String()
			}
		}
		if len(_iface) > 0 {
			_iface["name"] = iface.Name
			multiNetworkInfo = append(multiNetworkInfo, _iface)
		}
	}
	return multiNetworkInfo
}

type Ipv6Address struct{}

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

	// We don't return an error if no IPv6 interface has been found. Indeed,
	// some orgs just don't have IPv6 enabled. If there's a network error, it
	// will pop out when getting the Mac address and/or the IPv4 address
	// (before this function's call; see network.go -> getNetworkInfo())
	return "", nil
}

type IpAddress struct{}

func externalIpAddress() (string, error) {
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
			ip = ip.To4()
			if ip == nil {
				// not an ipv4 address
				continue
			}
			return ip.String(), nil
		}
	}
	return "", errors.New("not connected to the network")
}

type MacAddress struct{}

func macAddress() (string, error) {
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
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			return iface.HardwareAddr.String(), nil
		}
	}
	return "", errors.New("not connected to the network")
}
