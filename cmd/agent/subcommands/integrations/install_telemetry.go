// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package integrations

import (
	"encoding/json"
	"os"
	"time"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
)

const (
	// installEventName is registered in comp/core/agenttelemetry/impl/defaultProfiles.yaml.
	installEventName = "integration-install"
	// installEventTimeout bounds how long the command waits for the event to be sent.
	installEventTimeout = 3 * time.Second
	// downloaderResultFileFlag asks the downloader to write a downloaderResult to the given path.
	downloaderResultFileFlag = "--result-file"
)

// Steps of an install, reported as the stage of a failed install.
const (
	stageValidate      = "validate"
	stageDownload      = "download"
	stageCompatibility = "compatibility"
	stagePipInstall    = "pip_install"
	stageConfigMove    = "config_move"
)

// installEvent is the payload of the integration-install Agent telemetry event. Every field is an
// integration name and version or a value from a fixed set: it must never carry error messages,
// paths, or command output, which can contain customer data.
type installEvent struct {
	Integration string `json:"integration,omitempty"`
	Version     string `json:"version,omitempty"`
	// Source is core, extras, or local.
	Source string `json:"source"`
	// Result is success, failure, or noop when the version is already installed.
	Result string `json:"result"`
	// Stage is the step that failed, empty on success.
	Stage                     string `json:"stage,omitempty"`
	ErrorType                 string `json:"error_type,omitempty"`
	Repository                string `json:"repository,omitempty"`
	V2Failure                 string `json:"v2_failure,omitempty"`
	UnsafeDisableVerification bool   `json:"unsafe_disable_verification"`
	DurationMs                int64  `json:"duration_ms"`
}

func newInstallEvent(cliParams *cliParams) installEvent {
	source := "core"
	if cliParams.localWheel {
		source = "local"
	} else if cliParams.thirdParty {
		source = "extras"
	}
	return installEvent{
		Source:                    source,
		Stage:                     stageValidate,
		UnsafeDisableVerification: cliParams.unsafeDisableVerification,
	}
}

func (ev *installEvent) finish(err error, duration time.Duration) {
	ev.DurationMs = duration.Milliseconds()
	switch {
	case err != nil:
		ev.Result = "failure"
	case ev.Result == "":
		ev.Result = "success"
		ev.Stage = ""
	default:
		ev.Stage = ""
	}
}

// downloaderResult is the summary that the downloader writes to the path given with
// downloaderResultFileFlag.
type downloaderResult struct {
	Repository string `json:"repository"`
	V2Failure  string `json:"v2_failure"`
	ErrorType  string `json:"error_type"`
}

// readDownloaderResult returns the summary written by the downloader. A missing or malformed file,
// for example from a downloader that predates the result file, yields an empty summary.
func readDownloaderResult(path string) downloaderResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return downloaderResult{}
	}
	var result downloaderResult
	if err := json.Unmarshal(data, &result); err != nil {
		return downloaderResult{}
	}
	return result
}

// sendInstallEvent sends the event and waits for it for at most timeout, so that a slow or
// unreachable intake never holds the command for longer. An event still in flight is dropped when
// the process exits. Errors are ignored: telemetry must never fail an install.
func sendInstallEvent(atel agenttelemetry.Component, ev installEvent, timeout time.Duration) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = atel.SendEvent(installEventName, payload)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}
