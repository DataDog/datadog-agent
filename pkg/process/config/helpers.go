package config

import (
	"os"
)

// GetSocketPath exports the socket path we are using for the system probe
func GetSocketPath() string {
	path := defaultSystemProbeSocketPath
	if v, ok := os.LookupEnv("DD_SYSPROBE_SOCKET"); ok {
		path = v
	}

	return path
}
