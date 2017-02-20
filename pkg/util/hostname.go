package util

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

const maxLength = 255

var (
	validHostnameRfc1123 = regexp.MustCompile(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`)
	localhostIdentifiers = []string{
		"localhost",
		"localhost.localdomain",
		"localhost6.localdomain6",
		"ip6-localhost",
	}
)

// ValidHostname determines whether the passed string is a valid hostname.
// In case it's not, the returned error contains the details of the failure.
func ValidHostname(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("host name is empty")
	} else if isLocal(hostname) {
		return fmt.Errorf("%s is a local hostname", hostname)
	} else if len(hostname) > maxLength {
		return fmt.Errorf("name exceeded the maximum length of %d characters", maxLength)
	} else if !validHostnameRfc1123.MatchString(hostname) {
		return fmt.Errorf("%s is not RFC1123 compliant", hostname)
	}
	return nil
}

// check whether the name is in the list of local hostnames
func isLocal(name string) bool {
	name = strings.ToLower(name)
	for _, val := range localhostIdentifiers {
		if val == name {
			return true
		}
	}
	return false
}

// Fqdn returns the FQDN for the host if any
func Fqdn(hostname string) string {
	addrs, err := net.LookupIP(hostname)
	if err != nil {
		return hostname
	}

	for _, addr := range addrs {
		if ipv4 := addr.To4(); ipv4 != nil {
			ip, err := ipv4.MarshalText()
			if err != nil {
				return hostname
			}
			hosts, err := net.LookupAddr(string(ip))
			if err != nil || len(hosts) == 0 {
				return hostname
			}
			return hosts[0]
		}
	}
	return hostname
}
