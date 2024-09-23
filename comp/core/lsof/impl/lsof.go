// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package lsofimpl implements the lsof component interface
package lsofimpl

import (
	"errors"
	"runtime"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	lsofdef "github.com/DataDog/datadog-agent/comp/core/lsof/def"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/lsof"
)

// Requires defines the dependencies for the lsof component
type Requires struct{}

// Provides defines the output of the lsof component
type Provides struct {
	Comp          lsofdef.Component
	FlareProvider flaretypes.Provider
}

func fillFlare(fb flaretypes.FlareBuilder) error {
	if fb.IsLocal() {
		_ = fb.Logf("listing open files is not supported in local mode")
		return nil
	}

	files, err := lsof.ListOpenFilesFromSelf()
	if err != nil {
		if errors.Is(err, lsof.ErrNotImplemented) {
			_ = fb.Logf("listing files opened by the agent process is not supported on %s/%s", runtime.GOOS, runtime.GOARCH)
			return nil
		}
		_ = fb.Logf("could not list agent open files: %v", err)
		return err
	}

	return fb.AddFile(flavor.GetFlavor()+"_open_files.txt", []byte(files.String()))
}

// NewComponent creates a new lsof component
func NewComponent(Requires) (Provides, error) {
	provides := Provides{
		FlareProvider: flaretypes.NewProvider(fillFlare),
	}
	return provides, nil
}
