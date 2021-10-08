// +build !linux,!windows

package config

import "fmt"

const (
	defaultConfigDir          = ""
	defaultSystemProbeAddress = ""
)

// ValidateSocketAddress validates that the sysprobe socket config option is of the correct format.
func ValidateSocketAddress(sockPath string) error {
	return fmt.Errorf("system-probe unsupported")
}
