// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

// Since returns the date from when logs should be collected.
func Since(registry auditor.Registry, identifier string, creationTime service.CreationTime) (time.Time, error) {
	var since time.Time
	var err error
	offset := registry.GetOffset(identifier)
	switch {
	case offset != "":
		// an offset was registered, tail from the offset
		since, err = time.Parse(config.DateFormat, offset)
		if err != nil {
			since = time.Now().UTC()
		}
	case creationTime == service.After:
		// a new service has been discovered and was launched after the agent start, tail from the beginning
		since = time.Time{}
	case creationTime == service.Before:
		// a new config has been discovered and was launched before the agent start, tail from the end
		since = time.Now().UTC()
	}
	return since, err
}
