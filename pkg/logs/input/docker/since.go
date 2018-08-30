// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Since returns the date from when logs should be collected.
func Since(registry auditor.Registry, identifier string, tailFromBeginning bool) (time.Time, error) {
	var since time.Time
	var err error
	offset := registry.GetOffset(identifier)
	switch {
	case offset != "":
		// an offset was registered, tail from the offset
		since, err = time.Parse(config.DateFormat, offset)
		if err != nil {
			since = time.Now().UTC()
		} else {
			since = since.Add(time.Nanosecond)
		}
	case tailFromBeginning:
		// a new service has been discovered, tail from the beginning
		since = time.Time{}
	default:
		// a new config has been discovered, tail from the end
		since = time.Now().UTC()
	}
	return since, err
}
