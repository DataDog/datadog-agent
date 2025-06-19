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

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	apiutils "github.com/DataDog/datadog-agent/comp/api/api/utils"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	pkgFlare "github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// FlareBuilderFactory creates an instance of FlareBuilder
type flareBuilderFactory func(localFlare bool, flareArgs types.FlareArgs) (types.FlareBuilder, error)

var fbFactory flareBuilderFactory = helpers.NewFlareBuilder

type dependencies struct {
	fx.In

	Log       log.Component
	Config    config.Component
	Params    Params
	Providers []*types.FlareFiller `group:"flare"`
	WMeta     option.Option[workloadmeta.Component]
	IPC       ipc.Component
}

type provides struct {
	fx.Out

	Comp       Component
	Endpoint   api.AgentEndpointProvider
	RCListener rcclienttypes.TaskListenerProvider
}

type flare struct {
	log       log.Component
	config    config.Component
	params    Params
	providers []*types.FlareFiller
}

func newFlare(deps dependencies) provides {
	f := &flare{
		log:       deps.Log,
		config:    deps.Config,
		params:    deps.Params,
		providers: fxutil.GetAndFilterGroup(deps.Providers),
	}

	// Adding legacy and internal providers. Registering then as Provider through FX create cycle dependencies.
	//
	// Do not extend this list, this is legacy behavior that should be remove at some point. To add data to a flare
	// use the flare provider system: https://datadoghq.dev/datadog-agent/components/shared_features/flares/
	f.providers = append(
		f.providers,
		pkgFlare.ExtraFlareProviders(deps.WMeta, deps.IPC)...,
	)
	f.providers = append(
		f.providers,
		types.NewFiller(f.collectLogsFiles),
		types.NewFiller(f.collectConfigFiles),
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

	flareArgs := types.FlareArgs{}

	enableProfiling, found := task.Config.TaskArgs["enable_profiling"]
	if !found {
		f.log.Debug("enable_profiling arg not found, creating flare without profiling enabled")
	} else if enableProfiling == "true" {
		// RC expects the agent task operation to provide reasonable default flare args
		flareArgs.ProfileDuration = f.config.GetDuration("flare.rc_profiling.profile_duration")
		flareArgs.ProfileBlockingRate = f.config.GetInt("flare.rc_profiling.blocking_rate")
		flareArgs.ProfileMutexFraction = f.config.GetInt("flare.rc_profiling.mutex_fraction")
	} else if enableProfiling != "false" {
		f.log.Infof("Unrecognized value passed via enable_profiling, creating flare without profiling enabled: %q", enableProfiling)
	}

	streamlogs, found := task.Config.TaskArgs["enable_streamlogs"]
	if !found || streamlogs == "false" {
		f.log.Debug("enable_streamlogs arg not found, creating flare without streamlogs enabled")
	} else if streamlogs == "true" {
		flareArgs.StreamLogsDuration = f.config.GetDuration("flare.rc_streamlogs.duration")
	} else if streamlogs != "false" {
		f.log.Infof("Unrecognized value passed via enable_streamlogs, creating flare without streamlogs enabled: %q", streamlogs)
	}

	filePath, err := f.CreateWithArgs(flareArgs, 0, nil, []byte{})
	if err != nil {
		return true, err
	}

	f.log.Infof("Flare was created by remote-config at %s", filePath)

	_, err = f.Send(filePath, caseID, userHandle, helpers.NewRemoteConfigFlareSource(task.Config.UUID))
	return true, err
}

func (f *flare) createAndReturnFlarePath(w http.ResponseWriter, r *http.Request) {
	var profile types.ProfileData

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
	conn, ok := apiutils.GetConnection(r)
	if ok {
		_ = conn.SetDeadline(time.Time{})
	}

	var filePath string
	f.log.Infof("Making a flare")
	filePath, err := f.Create(profile, providerTimeout, nil, []byte{})

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
func (f *flare) Create(pdata types.ProfileData, providerTimeout time.Duration, ipcError error, diagnoseResult []byte) (string, error) {
	return f.create(types.FlareArgs{}, providerTimeout, ipcError, pdata, diagnoseResult)
}

// Create creates a new flare and returns the path to the final archive file.
//
// If providerTimeout is 0 or negative, the timeout from the configuration will be used.
func (f *flare) CreateWithArgs(flareArgs types.FlareArgs, providerTimeout time.Duration, ipcError error, diagnoseResult []byte) (string, error) {
	return f.create(flareArgs, providerTimeout, ipcError, types.ProfileData{}, diagnoseResult)
}

func (f *flare) create(flareArgs types.FlareArgs, providerTimeout time.Duration, ipcError error, pdata types.ProfileData, diagnoseResult []byte) (string, error) {
	if providerTimeout <= 0 {
		providerTimeout = f.config.GetDuration("flare_provider_timeout")
	}

	fb, err := fbFactory(f.params.local, flareArgs)
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

	if fb.IsLocal() {
		fb.AddFile("diagnose.log", diagnoseResult)
	}

	f.runProviders(fb, providerTimeout)

	return fb.Save()
}

func (f *flare) runProviders(fb types.FlareBuilder, providerTimeout time.Duration) {
	timer := time.NewTimer(providerTimeout)
	defer timer.Stop()

	for _, p := range f.providers {
		timeout := max(providerTimeout, p.Timeout(fb))
		timer.Reset(timeout)
		providerName := runtime.FuncForPC(reflect.ValueOf(p.Callback).Pointer()).Name()
		f.log.Infof("Running flare provider %s with timeout %s", providerName, timeout)
		_ = fb.Logf("Running flare provider %s with timeout %s", providerName, timeout)

		done := make(chan struct{})
		go func() {
			startTime := time.Now()
			err := p.Callback(fb)
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
			err := f.log.Warnf("flare provider '%s' skipped after %s", providerName, timeout)
			_ = fb.Logf("%s", err.Error())
		}
	}

	f.log.Info("All flare providers have been run, creating archive...")
}
