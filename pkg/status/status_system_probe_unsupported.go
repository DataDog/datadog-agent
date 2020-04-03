// +build zlib

package status

import (
	"fmt"
)

func getSystemProbeStats() map[string]interface{} {
	return map[string]interface{}{
		"Errors": fmt.Sprintf("System Probe Unsupported"),
	}
}
