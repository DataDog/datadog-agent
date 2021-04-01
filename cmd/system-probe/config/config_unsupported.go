// +build !linux,!windows

package config

import "fmt"

const (
	defaultConfigDir          = ""
	defaultSystemProbeAddress = ""
)

func ValidateSocketAddress(sockPath string) error {
	return fmt.Errorf("system-probe unsupported")
}
