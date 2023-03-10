// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"reflect"
	"runtime"

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
		err = p.Callback(fb)
		f.log.Error("error calling '%s' for flare creation: %s",
			runtime.FuncForPC(reflect.ValueOf(p.Callback).Pointer()).Name(), // reflect p.Callback function name
			err)
	}

	// Legacy flare code
	pkgFlare.CompleteFlare(fb, local, distPath, pyChecksPath, logFilePaths, pdata, ipcError)

	return fb.Save()
}
