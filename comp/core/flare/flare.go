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
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	apidef "github.com/DataDog/datadog-agent/comp/api/api/def"
	apiutils "github.com/DataDog/datadog-agent/comp/api/api/utils"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
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
	AC                    optional.Option[autodiscovery.Component]
}

type provides struct {
	fx.Out

	Comp     Component
	Endpoint apidef.AgentEndpointProvider
}

type flare struct {
	log          log.Component
	config       config.Component
	params       Params
	providers    []types.FlareCallback
	diagnoseDeps diagnose.SuitesDeps
}

func newFlare(deps dependencies) (provides, rcclienttypes.TaskListenerProvider) {
	diagnoseDeps := diagnose.NewSuitesDeps(deps.Diagnosesendermanager, deps.Collector, deps.Secrets, deps.WMeta, deps.AC)
	f := &flare{
		log:          deps.Log,
		config:       deps.Config,
		params:       deps.Params,
		providers:    fxutil.GetAndFilterGroup(deps.Providers),
		diagnoseDeps: diagnoseDeps,
	}

	p := provides{
		Comp:     f,
		Endpoint: apidef.NewAgentEndpointProvider(f.createAndReturnFlarePath, "/flare", "POST"),
	}

	return p, rcclienttypes.NewTaskListener(f.onAgentTaskEvent)
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

	filePath, err := f.Create(nil, nil)
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

	// Reset the `server_timeout` deadline for this connection as creating a flare can take some time
	conn := apiutils.GetConnection(r)
	_ = conn.SetDeadline(time.Time{})

	var filePath string
	var err error
	f.log.Infof("Making a flare")
	filePath, err = f.Create(profile, nil)

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
		fb.AddFile("local", msg) //nolint:errcheck
	}

	for name, data := range pdata {
		fb.AddFileWithoutScrubbing(filepath.Join("profiles", name), data) //nolint:errcheck
	}

	// Adding legacy and internal providers. Registering then as Provider through FX create cycle dependencies.
	providers := append(
		f.providers,
		func(fb types.FlareBuilder) error {
			return pkgFlare.CompleteFlare(fb, f.diagnoseDeps)
		},
		f.collectLogsFiles,
		f.collectConfigFiles,
	)

	for _, p := range providers {
		err = p(fb)
		if err != nil {
			f.log.Errorf("error calling '%s' for flare creation: %s",
				runtime.FuncForPC(reflect.ValueOf(p).Pointer()).Name(), // reflect p.Callback function name
				err)
		}
	}

	return fb.Save()
}
