package network

import (
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
