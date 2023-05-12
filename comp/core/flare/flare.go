// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	pkgFlare "github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// ProfileData maps (pprof) profile names to the profile data.
type ProfileData map[string][]byte

type dependencies struct {
	fx.In

	Log       log.Component
	Config    config.Component
	Params    Params
	Providers []helpers.FlareProvider `group:"flare"`
}

type flare struct {
	log       log.Component
	config    config.Component
	params    Params
	providers []helpers.FlareProvider
	rcClient  *remote.Client
}

func newFlare(deps dependencies) (Component, error) {
	f := &flare{
		log:       deps.Log,
		config:    deps.Config,
		params:    deps.Params,
		providers: deps.Providers,
	}

	c, err := remote.NewUnverifiedGRPCClient(
		"flare-comp", version.AgentVersion, []data.Product{data.ProductAgentTask}, 2*time.Second,
	)

	// If there is an error we consider RC is not started
	if err == nil {
		f.rcClient = c

		f.rcClient.RegisterAgentTaskUpdate(f.onAgentTaskEvent)

		f.rcClient.Start()
	}

	return f, nil
}

func (f *flare) onAgentTaskEvent(configs map[string]state.AgentTaskConfig) {
	f.log.Warnf("[RCM] Creating flare based on the agent task")
	path, err := f.Create(nil, nil)

	f.log.Warnf("[RCM] Flare created in %s based on the agent task: %v", path, err)
}

// Send sends a flare archive to Datadog
func (f *flare) Send(flarePath string, caseID string, email string) (string, error) {
	// For now this is a wrapper around helpers.SendFlare since some code hasn't migrated to FX yet.
	return helpers.SendTo(flarePath, caseID, email, f.config.GetString("api_key"), utils.GetInfraEndpoint(f.config))
}

// Create creates a new flare and returns the path to the final archive file.
func (f *flare) Create(pdata ProfileData, ipcError error) (string, error) {
	fb, err := helpers.NewFlareBuilder(f.params.local)
	if err != nil {
		return "", err
	}

	if fb.IsLocal() {
		// If we have a ipcError we failed to reach the agent process, else the user requested a local flare
		// from the CLI.
		msg := []byte("local flare was requested")
		if ipcError != nil {
			msg = []byte(fmt.Sprintf("unable to contact the agent to retrieve flare: %s", ipcError))
		}
		fb.AddFile("local", msg)
	}

	for name, data := range pdata {
		fb.AddFileWithoutScrubbing(filepath.Join("profiles", name), data)
	}

	// Adding legacy and internal providers. Registering then as FlareProvider through FX create cycle dependencies.
	providers := append(
		f.providers,
		helpers.FlareProvider{Callback: pkgFlare.CompleteFlare},
		helpers.FlareProvider{Callback: f.collectLogsFiles},
		helpers.FlareProvider{Callback: f.collectConfigFiles},
	)

	for _, p := range providers {
		err = p.Callback(fb)
		if err != nil {
			f.log.Errorf("error calling '%s' for flare creation: %s",
				runtime.FuncForPC(reflect.ValueOf(p.Callback).Pointer()).Name(), // reflect p.Callback function name
				err)
		}
	}

	return fb.Save()
}
