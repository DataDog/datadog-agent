// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package pidimpl writes the current PID to a file, ensuring that the file
// doesn't exist or doesn't contain a PID for a running process.
package pidimpl

import (
	"context"
	"os"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pid "github.com/DataDog/datadog-agent/comp/core/pid/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
)

// Params are the input parameters for the component.
type Params struct {
	PIDfilePath string
}

// NewParams returns a new Params with the given values.
func NewParams(pidfilePath string) Params {
	return Params{
		PIDfilePath: pidfilePath,
	}
}

// Requires defines the dependencies for the pid component.
type Requires struct {
	Lc     compdef.Lifecycle
	Log    log.Component
	Params Params
}

// Provides defines the output of the pid component.
type Provides struct {
	Comp pid.Component
}

// NewComponent creates a new pid component.
func NewComponent(reqs Requires) (Provides, error) {
	pidfilePath := reqs.Params.PIDfilePath
	if pidfilePath != "" {
		err := pidfile.WritePID(pidfilePath)
		if err != nil {
			return Provides{}, reqs.Log.Errorf("Error while writing PID file, exiting: %v", err)
		}
		reqs.Log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), pidfilePath)

		reqs.Lc.Append(compdef.Hook{
			OnStop: func(context.Context) error {
				err = os.Remove(pidfilePath)
				if err != nil {
					reqs.Log.Errorf("Error while removing PID file: %v", err)
				} else {
					reqs.Log.Infof("Removed PID file: %s", pidfilePath)
				}
				return nil
			},
		})
	}
	return Provides{Comp: struct{}{}}, nil
}
