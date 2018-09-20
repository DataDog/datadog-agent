// +build windows

package procdiscovery

import "errors"

// pollProcesses retrieves processes running on the host
func pollProcesses() ([]process, error) {
	return nil, errors.New("Not implemented yet on windows")
}
