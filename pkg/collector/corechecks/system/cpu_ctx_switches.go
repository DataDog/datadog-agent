// +build !linux

package system

func (c *CPUCheck) collectCtxSwitches() error {
	// On non-linux systems, do nothing
	return nil
}
