// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgFlare "github.com/DataDog/datadog-agent/pkg/flare"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In

	Log       log.Component
	Providers []helpers.FlareProvider `group:"flare"`
}

type flare struct {
	log       log.Component
	providers []helpers.FlareProvider
}

func newFlare(deps dependencies) (Component, error) {
	return &flare{
		log:       deps.Log,
		providers: deps.Providers,
	}, nil
}

func (f *flare) Create(local bool, distPath, pyChecksPath string, logFilePaths []string, pdata pkgFlare.ProfileData, ipcError error) (string, error) {
	fb, err := helpers.NewFlareBuilder()
	if err != nil {
		return "", err
	}

	for _, p := range f.providers {
		p.Callback(fb) //nolint:errcheck
	}

	// Legacy flare code
	pkgFlare.CompleteFlare(fb, local, distPath, pyChecksPath, logFilePaths, pdata, ipcError)

	return fb.Save()
}

func newMock(deps dependencies, t testing.TB) Component {
	return &flare{}
}
