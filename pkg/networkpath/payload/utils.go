// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package payload

import (
	"errors"
	"fmt"
)

// ValidateNetworkPath validates a NetworkPath payload.
// Returns an error if any run does not have a valid destination IP address.
func ValidateNetworkPath(path *NetworkPath) error {
	if path == nil {
		return errors.New("invalid nil path")
	}

	if len(path.Traceroute.Runs) == 0 {
		return nil
	}

	for i, run := range path.Traceroute.Runs {
		// IP address will be nil if json.Unmarshal fails to parse an invalid IP address
		// from the traceroute results returned by system-probe
		if len(run.Destination.IPAddress) == 0 {
			return fmt.Errorf("traceroute run %d (%s) has invalid destination IP address for %s:%d", i, run.RunID, path.Destination.Hostname, path.Destination.Port)
		}
	}

	return nil
}
