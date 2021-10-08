// +build !linux

package kernel

// IsIPv6Enabled returns whether or not IPv6 has been enabled on the host
func IsIPv6Enabled() bool {
	return true
}
