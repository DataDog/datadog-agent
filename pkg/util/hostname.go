package util

import (
	"fmt"
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
