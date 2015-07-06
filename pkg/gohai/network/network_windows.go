package network

import (
	"errors"
	"os/exec"
	"strings"
)

func getNetworkInfo() (networkInfo map[string]interface{}, err error) {
	networkInfo = make(map[string]interface{})

	out, err := exec.Command("ipconfig", "-all").CombinedOutput()
	if err != nil {
		return
	}

	networkInfo, err = parseIpConfig(string(out))
	return
}

func parseIpConfig(out string) (networkInfo map[string]interface{}, err error) {
	// The hardest part is that we want the 3 addresses to come from the same block
	// or else, it wouldn't really make sense. Also we assume that only one
	// interface is seriously enabled and has IPv4 at least
	networkInfo = make(map[string]interface{})
	var ip, mac, ipv6 string

	lines := strings.Split(string(out), "\n")
	gottablock := false
	for _, line := range lines {
		// The line below is here in case we have to convert some Unicode to ASCII
		// It shouldn't do anything on Windows but when running the tests (for
		// Windows) on a Unix based-system, it's essential.
		line = strings.Replace(line, "\x00", "", -1)

		if strings.Contains(line, "IPv4") {
			ip = line
			gottablock = true
		} else if strings.Contains(line, "Physical Address") && mac == "" {
			mac = line
		} else if strings.Contains(line, "IPv6") && ipv6 == "" {
			ipv6 = line
		}
		// Whenever we reach the end of a block
		if isEmptyString(line) {
			if gottablock && mac != "" {
				break
			} else { // Or something's wrong... let's try again with the next block
				gottablock = false
				ip, mac, ipv6 = "", "", ""
			}
		}
	}

	elt := strings.Split(ip, ": ")
	if len(elt) >= 2 {
		networkInfo["ipaddress"] = strings.Trim(strings.Trim(elt[1], "\r"), "(Preferred) ")
	} else {
		return networkInfo, errors.New("not connected to the network")
	}

	// We're sure to have a mac address at this point, no paranoia check needed
	elt = strings.Split(mac, ": ")
	networkInfo["macaddress"] = strings.Trim(strings.Trim(elt[1], "\r"), "(Preferred) ")

	// But some interfaces still don't like IPv6 (or have it turned off)
	elt = strings.Split(ipv6, ": ")
	if len(elt) >= 2 {
		networkInfo["ipaddressv6"] = strings.Replace(strings.Trim(elt[1], "\r"), "(Preferred) ", "", -1)
	} else {
		networkInfo["ipaddressv6"] = ""
	}
	return
}

func isEmptyString(val string) bool {
	return val == "\r" || val == ""
}
