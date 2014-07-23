package network

import (
	"errors"
	"os/exec"
	"strings"
)

func getNetworkInfo() (networkInfo map[string]interface{}, err error) {
	networkInfo = make(map[string]interface{})
	var ip, mac, ipv6 string

	out, err := exec.Command("ipconfig", "-all").CombinedOutput()
	if err != nil {
		return networkInfo, err
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "IPv4") && ip == "" {
			ip = line
		} else if strings.Contains(line, "Physical Address") && mac == "" {
			mac = line
		} else if strings.Contains(line, "IPv6") && ipv6 == "" {
			ipv6 = line
		}
	}

	elt := strings.Split(ip, ": ")
	if len(elt) >= 2 {
		networkInfo["ipaddress"] = strings.Trim(strings.Trim(elt[1], "\r"), "(Preferred) ")
	} else {
		return networkInfo, errors.New("not connected to the network")
	}

	elt = strings.Split(mac, ": ")
	networkInfo["macaddress"] = strings.Trim(strings.Trim(elt[1], "\r"), "(Preferred) ")
	elt = strings.Split(ipv6, ": ")
	networkInfo["ipaddressv6"] = strings.Trim(strings.Trim(elt[1], "\r"), "(Preferred) ")

	return
}
