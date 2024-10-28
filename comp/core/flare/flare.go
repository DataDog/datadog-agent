// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	apiutils "github.com/DataDog/datadog-agent/comp/api/api/utils"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	pkgFlare "github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// ProfileData maps (pprof) profile names to the profile data.
type ProfileData map[string][]byte

type dependencies struct {
	fx.In

	Log                   log.Component
	Config                config.Component
	Diagnosesendermanager diagnosesendermanager.Component
	Params                Params
	Providers             []types.FlareCallback `group:"flare"`
	Collector             optional.Option[collector.Component]
	WMeta                 optional.Option[workloadmeta.Component]
	Secrets               secrets.Component
	AC                    autodiscovery.Component
	Tagger                tagger.Component
}

type provides struct {
	fx.Out

	Comp       Component
	Endpoint   api.AgentEndpointProvider
	RCListener rcclienttypes.TaskListenerProvider
}

type flare struct {
	log          log.Component
	config       config.Component
	params       Params
	providers    []types.FlareCallback
	diagnoseDeps diagnose.SuitesDeps
}

func newFlare(deps dependencies) provides {
	diagnoseDeps := diagnose.NewSuitesDeps(deps.Diagnosesendermanager, deps.Collector, deps.Secrets, deps.WMeta, deps.AC, deps.Tagger)
	f := &flare{
		log:          deps.Log,
		config:       deps.Config,
		params:       deps.Params,
		providers:    fxutil.GetAndFilterGroup(deps.Providers),
		diagnoseDeps: diagnoseDeps,
	}

	// Adding legacy and internal providers. Registering then as Provider through FX create cycle dependencies.
	//
	// Do not extend this list, this is legacy behavior that should be remove at some point. To add data to a flare
	// use the flare provider system: https://datadoghq.dev/datadog-agent/components/shared_features/flares/
	f.providers = append(
		f.providers,
		pkgFlare.ExtraFlareProviders(f.diagnoseDeps)...,
	)
	f.providers = append(
		f.providers,
		f.collectLogsFiles,
		f.collectConfigFiles,
	)

	return provides{
		Comp:       f,
		Endpoint:   api.NewAgentEndpointProvider(f.createAndReturnFlarePath, "/flare", "POST"),
		RCListener: rcclienttypes.NewTaskListener(f.onAgentTaskEvent),
	}
}

func (f *flare) onAgentTaskEvent(taskType rcclienttypes.TaskType, task rcclienttypes.AgentTaskConfig) (bool, error) {
	if taskType != rcclienttypes.TaskFlare {
		return false, nil
	}
	caseID, found := task.Config.TaskArgs["case_id"]
	if !found {
		return true, fmt.Errorf("Case ID was not provided in the flare agent task")
	}
	userHandle, found := task.Config.TaskArgs["user_handle"]
	if !found {
		return true, fmt.Errorf("User handle was not provided in the flare agent task")
	}

	filePath, err := f.Create(nil, 0, nil)
	if err != nil {
		return true, err
	}

	f.log.Infof("Flare was created by remote-config at %s", filePath)

	_, err = f.Send(filePath, caseID, userHandle, helpers.NewRemoteConfigFlareSource(task.Config.UUID))
	return true, err
}

func (f *flare) createAndReturnFlarePath(w http.ResponseWriter, r *http.Request) {
	var profile ProfileData

	if r.Body != http.NoBody {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, f.log.Errorf("Error while reading HTTP request body: %s", err).Error(), 500)
			return
		}

		if err := json.Unmarshal(body, &profile); err != nil {
			http.Error(w, f.log.Errorf("Error while unmarshaling JSON from request body: %s", err).Error(), 500)
			return
		}
	}

	var providerTimeout time.Duration

	queryProviderTimeout := r.URL.Query().Get("provider_timeout")
	if queryProviderTimeout != "" {
		givenTimeout, err := strconv.ParseInt(queryProviderTimeout, 10, 64)
		if err == nil && givenTimeout > 0 {
			providerTimeout = time.Duration(givenTimeout)
		} else {
			f.log.Warnf("provider_timeout query parameter must be a positive integer, but was %s, using configuration value", queryProviderTimeout)
		}
	}

	// Reset the `server_timeout` deadline for this connection as creating a flare can take some time
	conn := apiutils.GetConnection(r)
	_ = conn.SetDeadline(time.Time{})

	var filePath string
	f.log.Infof("Making a flare")
	filePath, err := f.Create(profile, providerTimeout, nil)

	if err != nil || filePath == "" {
		if err != nil {
			f.log.Errorf("The flare failed to be created: %s", err)
		} else {
			f.log.Warnf("The flare failed to be created")
		}
		http.Error(w, err.Error(), 500)
	}
	w.Write([]byte(filePath))
}

// Send sends a flare archive to Datadog
func (f *flare) Send(flarePath string, caseID string, email string, source helpers.FlareSource) (string, error) {
	// For now this is a wrapper around helpers.SendFlare since some code hasn't migrated to FX yet.
	// The `source` is the reason why the flare was created, for now it's either local or remote-config
	return helpers.SendTo(f.config, flarePath, caseID, email, f.config.GetString("api_key"), utils.GetInfraEndpoint(f.config), source)
}

// Create creates a new flare and returns the path to the final archive file.
//
// If providerTimeout is 0 or negative, the timeout from the configuration will be used.
func (f *flare) Create(pdata ProfileData, providerTimeout time.Duration, ipcError error) (string, error) {
	if providerTimeout <= 0 {
		providerTimeout = f.config.GetDuration("flare_provider_timeout")
	}

	fb, err := helpers.NewFlareBuilder(f.params.local)
	if err != nil {
		return "", err
	}

	fb.Logf("Flare creation time: %s", time.Now().Format(time.RFC3339)) //nolint:errcheck
	if fb.IsLocal() {
		// If we have a ipcError we failed to reach the agent process, else the user requested a local flare
		// from the CLI.
		msg := "local flare was requested"
		if ipcError != nil {
			msg = fmt.Sprintf("unable to contact the agent to retrieve flare: %s", ipcError)
		}
		fb.AddFile("local", []byte(msg)) //nolint:errcheck
	}

	for name, data := range pdata {
		fb.AddFileWithoutScrubbing(filepath.Join("profiles", name), data) //nolint:errcheck
	}

	f.runProviders(fb, providerTimeout)

	return fb.Save()
}

func (f *flare) runProviders(fb types.FlareBuilder, providerTimeout time.Duration) {
	timer := time.NewTimer(providerTimeout)
	defer timer.Stop()

	for _, p := range f.providers {
		providerName := runtime.FuncForPC(reflect.ValueOf(p).Pointer()).Name()
		f.log.Infof("Running flare provider %s", providerName)
		_ = fb.Logf("Running flare provider %s", providerName)

		done := make(chan struct{})
		go func() {
			startTime := time.Now()
			err := p(fb)
			duration := time.Since(startTime)

			if err == nil {
				f.log.Debugf("flare provider '%s' completed in %s", providerName, duration)
			} else {
				errMsg := f.log.Errorf("flare provider '%s' failed after %s: %s", providerName, duration, err)
				_ = fb.Logf("%s", errMsg.Error())
			}

			done <- struct{}{}
		}()

		select {
		case <-done:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			err := f.log.Warnf("flare provider '%s' skipped after %s", providerName, providerTimeout)
			_ = fb.Logf("%s", err.Error())
		}
		timer.Reset(providerTimeout)
	}

	f.log.Info("All flare providers have been run, creating archive...")
}
