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
	return
}

func getNetworkInfo() (networkInfo map[string]interface{}, err error) {
	networkInfo = make(map[string]interface{})

	macaddress, err := macAddress()
	if err != nil {
		return networkInfo, err
	}
	networkInfo["macaddress"] = macaddress

	ipAddress, err := externalIpAddress()
	if err != nil {
		return networkInfo, err
	}
	networkInfo["ipaddress"] = ipAddress

	ipAddressV6, err := externalIpv6Address()
	if err != nil {
		return networkInfo, err
	}
	networkInfo["ipaddressv6"] = ipAddressV6

	return
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
	return "", errors.New("not connected to the network")
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


/* Not currently used */

func getDetailedNetworkInfo() (networkInfo map[string]interface{}, err error) {
	ifaces, err := net.Interfaces()

	if err != nil {
		return
	}

	networkInfo = make(map[string]interface{})
	var interfaces = make(map[string]interface{})
	networkInfo["interfaces"] = interfaces

	for _, iface := range ifaces {
		newIface := make(map[string]interface{})
		flags := getFlags(iface)
		addresses, _ := getAddresses(iface)
		newIface["mtu"] = iface.MTU
		newIface["flags"] = flags
		newIface["addresses"] = addresses
		interfaces[iface.Name] = newIface
	}

	return
}

func getFlags(iface net.Interface) (flags []string) {
	if iface.Flags&net.FlagUp != 0 {
		flags = append(flags, "UP")
	}
	if iface.Flags&net.FlagLoopback != 0 {
		flags = append(flags, "LOOPBACK")
	}
	if iface.Flags&net.FlagBroadcast != 0 {
		flags = append(flags, "BROADCASE")
	}
	if iface.Flags&net.FlagMulticast != 0 {
		flags = append(flags, "MULTICAST")
	}
	if iface.Flags&net.FlagPointToPoint != 0 {
		flags = append(flags, "POINTTOPOINT")
	}

	return
}

func getAddresses(iface net.Interface) (addresses map[string]interface{}, err error) {
	addresses = make(map[string]interface{})

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		var ip net.IP
		addressInfo := make(map[string]string)
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}

		netmask := ip.DefaultMask()
		if netmask != nil {
			addressInfo["family"] = "inet"
		} else {
			addressInfo["family"] = "inet6"
		}
		addresses[ip.String()] = addressInfo
	}

	mac := iface.HardwareAddr
	if mac != nil {
		addressInfo := make(map[string]string)
		addressInfo["family"] = "lladdr"
		addresses[mac.String()] = addressInfo
	}

	return
}
