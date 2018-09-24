// +build windows

package procdiscovery

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

// pollProcesses retrieves processes running on the host
func pollProcesses() ([]process, error) {
	counter, err := pdhutil.GetCounterSet("Process", "ID Process", "", nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't get counter set: %s", err)
	}

	values, err := counter.GetAllValues()
	if err != nil {
		return nil, fmt.Errorf("couldn't get counter values: %s", err)
	}

	pl := make([]process, 0, len(values))

	// Not really the process CLI but it seems there is no way to get it using PDH atm
	for name, pid := range values {
		pl = append(pl, process{cmd: name, pid: int32(pid)})
	}

	return pl, nil
}
