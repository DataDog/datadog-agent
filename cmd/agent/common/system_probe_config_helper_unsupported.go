// +build !process windows

package common

// SetupSystemProbeConfig returns nil on unsupported builds
func SetupSystemProbeConfig(sysProbeConfFilePath string) error {
	return nil
}
