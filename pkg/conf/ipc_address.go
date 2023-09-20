package conf

import (
	"fmt"
	"net"
)

// IsLocalAddress determines whether it is local address
func IsLocalAddress(address string) (string, error) {
	if address == "localhost" {
		return address, nil
	}
	ip := net.ParseIP(address)
	if ip == nil {
		return "", fmt.Errorf("address was set to an invalid IP address: %s", address)
	}
	for _, cidr := range []string{
		"127.0.0.0/8", // IPv4 loopback
		"::1/128",     // IPv6 loopback
	} {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", err
		}
		if block.Contains(ip) {
			return address, nil
		}
	}
	return "", fmt.Errorf("address was set to a non-loopback IP address: %s", address)
}

// GetIPCAddress returns the IPC address or an error if the address is not local
func GetIPCAddress(cfg Config) (string, error) {
	address, err := IsLocalAddress(cfg.GetString("ipc_address"))
	if err != nil {
		return "", fmt.Errorf("ipc_address: %s", err)
	}
	return address, nil
}
