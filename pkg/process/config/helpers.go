package config

// GetSocketPath exports the socket path we are using for the system probe.
func GetSocketPath() string {
	return defaultSystemProbeAddress
}

// LoadSysProbeEnvVariables will set the environment variables specific to the system probe
func LoadSysProbeEnvVariables() {
	loadSysProbeEnvVariables()
}
