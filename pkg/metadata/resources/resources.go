// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build linux windows darwin

package resources

import (
	"github.com/DataDog/gohai/processes"
	log "github.com/cihub/seelog"
)

// GetPayload builds a payload of processes metadata collected from gohai.
func GetPayload(hostname string) *Payload {

	// Get processes metadata from gohai
	proc, err := new(processes.Processes).Collect()
	if err != nil {
		log.Errorf("Failed to retrieve processes metadata: %s", err)
		return &Payload{}
	}

	processesPayload := map[string]interface{}{
		"snaps": []interface{}{proc},
	}

	return &Payload{
		Processes: processesPayload,
		Meta: map[string]string{
			"host": hostname,
		},
	}
}
