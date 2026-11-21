// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package integrations

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"time"

	"github.com/coreos/go-semver/semver"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
)

const (
	// installEventName is registered in comp/core/agenttelemetry/impl/defaultProfiles.yaml.
	installEventName = "integration-install"
	// installEventSchemaVersion is increased when the payload changes.
	installEventSchemaVersion = 1
	// installEventTimeout bounds how long the command waits for the event to be sent.
	installEventTimeout = 3 * time.Second
	// downloaderResultFileFlag asks the downloader to write a downloaderResult to the given path.
	downloaderResultFileFlag = "--result-file"
)

// Sources of the integration.
const (
	sourceCore   = "core"
	sourceExtras = "extras"
	sourceLocal  = "local"
)

// Results of an install. Every result but resultSuccess comes with a non-zero exit code.
const (
	resultSuccess = "success"
	// resultUserError is reported when the cause is certainly the customer's.
	resultUserError = "user_error"
	resultFailure   = "failure"
)

// Steps of an install, reported as the step that failed.
const (
	stepValidate      = "validate"
	stepDownload      = "download"
	stepCompatibility = "compatibility"
	stepPipInstall    = "pip_install"
	stepConfigMove    = "config_move"
)

// Error categories, a fixed list per step.
const (
	categoryInvalidArguments     = "invalid_arguments"
	categoryNotDatadogPackage    = "not_datadog_package"
	categoryBasePackageRefused   = "base_package_refused"
	categoryUserNotAllowed       = "user_not_allowed"
	categoryVersionBelowShipped  = "version_below_shipped"
	categoryLocalWheelUnreadable = "local_wheel_unreadable"
	categoryEnvironmentError     = "environment_error"

	// The download categories other than categoryDownloaderNoResult are reported by the downloader,
	// in its download_error_category function.
	categoryNotFound           = "not_found"
	categoryNetwork            = "network"
	categoryIntegrity          = "integrity"
	categoryMetadata           = "metadata"
	categoryVerification       = "verification"
	categoryDownloaderNoResult = "downloader_no_result"

	categoryNotAgentCheck           = "not_agent_check"
	categoryBaseVersionIncompatible = "base_version_incompatible"
	categoryWheelUnreadable         = "wheel_unreadable"

	categoryPipFailed = "pip_failed"

	categoryConfigMoveFailed = "config_move_failed"

	// categoryOther is reported for any unexpected error, in the step that was running.
	categoryOther = "other"
)

// categorySteps maps each error category but categoryOther to the steps it belongs to. A category
// of several steps is reported in the step that was running, or else in the first one.
var categorySteps = map[string][]string{
	categoryInvalidArguments:        {stepValidate},
	categoryNotDatadogPackage:       {stepValidate},
	categoryBasePackageRefused:      {stepValidate},
	categoryUserNotAllowed:          {stepValidate},
	categoryVersionBelowShipped:     {stepValidate},
	categoryLocalWheelUnreadable:    {stepValidate},
	categoryEnvironmentError:        {stepValidate, stepCompatibility},
	categoryNotFound:                {stepDownload},
	categoryNetwork:                 {stepDownload},
	categoryIntegrity:               {stepDownload},
	categoryMetadata:                {stepDownload},
	categoryVerification:            {stepDownload},
	categoryDownloaderNoResult:      {stepDownload},
	categoryNotAgentCheck:           {stepCompatibility},
	categoryBaseVersionIncompatible: {stepCompatibility},
	categoryWheelUnreadable:         {stepCompatibility},
	categoryPipFailed:               {stepPipInstall},
	categoryConfigMoveFailed:        {stepConfigMove},
}

// userErrorCategories are the categories whose cause is certainly the customer's. Monitors count
// every other category as a failure, including those that can come from either side.
var userErrorCategories = map[string]bool{
	categoryInvalidArguments:     true,
	categoryNotDatadogPackage:    true,
	categoryBasePackageRefused:   true,
	categoryUserNotAllowed:       true,
	categoryVersionBelowShipped:  true,
	categoryLocalWheelUnreadable: true,
	categoryConfigMoveFailed:     true,
}

// Install types, set once the installed version is known.
const (
	installTypeNew       = "new"
	installTypeUpgrade   = "upgrade"
	installTypeDowngrade = "downgrade"
	installTypeUnchanged = "unchanged"
)

// installEvent is the payload of the integration-install Agent telemetry event. Every field is an
// integration name and version, a path within one of our repositories, a duration, or a value from
// a fixed set: it must never carry error messages, host paths, or command output, which can contain
// customer data.
type installEvent struct {
	SchemaVersion              int    `json:"schema_version"`
	IntegrationName            string `json:"integration_name,omitempty"`
	IntegrationVersion         string `json:"integration_version,omitempty"`
	IntegrationSource          string `json:"integration_source"`
	Result                     string `json:"result"`
	FailedStep                 string `json:"failed_step,omitempty"`
	ErrorCategory              string `json:"error_category,omitempty"`
	UnsafeDisableVerification  bool   `json:"unsafe_disable_verification"`
	InstallType                string `json:"install_type,omitempty"`
	PreviousIntegrationVersion string `json:"previous_integration_version,omitempty"`
	DurationMs                 int64  `json:"duration_ms"`
	// Download is set once the download step starts.
	Download *downloadSummary `json:"download,omitempty"`

	// step is the step that is running, reported as the failed step of an uncategorized error.
	step string
}

// downloadSummary is what happened in the download step.
type downloadSummary struct {
	DownloaderVersion string             `json:"downloader_version,omitempty"`
	WheelPath         string             `json:"wheel_path,omitempty"`
	DurationMs        int64              `json:"duration_ms"`
	V2                *repositoryAttempt `json:"v2,omitempty"`
	V1                *repositoryAttempt `json:"v1,omitempty"`
}

// repositoryAttempt is what happened on one wheel repository, as reported by the downloader.
type repositoryAttempt struct {
	// Result is success, failure, or skipped when the repository wasn't tried.
	Result string `json:"result"`
	// ExceptionClass is the class of the downloader's exception on failure.
	ExceptionClass string `json:"exception_class,omitempty"`
	// DurationMs is unset when the repository was skipped.
	DurationMs *int64 `json:"duration_ms,omitempty"`
}

// downloaderResult is the summary that the downloader writes to the path given with
// downloaderResultFileFlag.
type downloaderResult struct {
	DownloaderVersion string             `json:"downloader_version"`
	WheelPath         string             `json:"wheel_path"`
	ErrorCategory     string             `json:"error_category"`
	V2                *repositoryAttempt `json:"v2"`
	V1                *repositoryAttempt `json:"v1"`
}

func newInstallEvent(cliParams *cliParams) installEvent {
	source := sourceCore
	if cliParams.localWheel {
		source = sourceLocal
	} else if cliParams.thirdParty {
		source = sourceExtras
	}
	return installEvent{
		SchemaVersion:             installEventSchemaVersion,
		IntegrationSource:         source,
		UnsafeDisableVerification: cliParams.unsafeDisableVerification,
		step:                      stepValidate,
	}
}

// recordDownload records the download step, from the downloader's summary when it wrote one.
func (ev *installEvent) recordDownload(result *downloaderResult, duration time.Duration) {
	ev.Download = &downloadSummary{DurationMs: duration.Milliseconds()}
	if result != nil {
		ev.Download.DownloaderVersion = result.DownloaderVersion
		ev.Download.WheelPath = result.WheelPath
		ev.Download.V2 = result.V2
		ev.Download.V1 = result.V1
	}
}

// finish records the outcome of the install, from the error returned by runInstall.
func (ev *installEvent) finish(err error, duration time.Duration) {
	ev.DurationMs = duration.Milliseconds()
	// A local wheel can be a customer's own check, so its name is never reported.
	if ev.IntegrationSource == sourceLocal {
		ev.IntegrationName, ev.IntegrationVersion = "", ""
	}
	if err == nil {
		ev.Result = resultSuccess
		return
	}
	ev.ErrorCategory = errorCategory(err)
	ev.FailedStep = ev.step
	if steps, ok := categorySteps[ev.ErrorCategory]; ok && !slices.Contains(steps, ev.step) {
		ev.FailedStep = steps[0]
	}
	ev.Result = resultFor(ev.ErrorCategory, ev.IntegrationSource)
}

func resultFor(category, source string) string {
	// A local wheel is the customer's own, while a published one that isn't a check is our mistake.
	if userErrorCategories[category] || (category == categoryNotAgentCheck && source == sourceLocal) {
		return resultUserError
	}
	return resultFailure
}

func installTypeFor(toInstall, current *semver.Version, found bool) string {
	switch {
	case !found:
		return installTypeNew
	case toInstall.Equal(*current):
		return installTypeUnchanged
	case toInstall.LessThan(*current):
		return installTypeDowngrade
	default:
		return installTypeUpgrade
	}
}

// categorizedError is an install error with the category that the event reports. Its message is
// that of the wrapped error, which the command prints.
type categorizedError struct {
	category string
	err      error
}

func (e *categorizedError) Error() string { return e.err.Error() }

func (e *categorizedError) Unwrap() error { return e.err }

func withCategory(category string, err error) error {
	return &categorizedError{category: category, err: err}
}

func errorCategory(err error) string {
	var categorized *categorizedError
	if errors.As(err, &categorized) {
		return categorized.category
	}
	return categoryOther
}

// downloadCategory returns the category of a failed download from the downloader's summary. Only
// the download categories are accepted from the downloader, which can be upgraded separately.
func downloadCategory(result *downloaderResult) string {
	if result == nil {
		return categoryDownloaderNoResult
	}
	if slices.Contains(categorySteps[result.ErrorCategory], stepDownload) {
		return result.ErrorCategory
	}
	return categoryOther
}

// baseIncompatibilityCategory tells a wheel that doesn't depend on datadog-checks-base, and so isn't
// an Agent check, from one that requires another version of it.
func baseIncompatibilityCategory(wheelPath string) string {
	dependsOnBase, err := validateBaseDependency(wheelPath, nil)
	switch {
	case err != nil:
		return categoryWheelUnreadable
	case !dependsOnBase:
		return categoryNotAgentCheck
	default:
		return categoryBaseVersionIncompatible
	}
}

// readDownloaderResult returns the summary written by the downloader, or nil if the file is
// missing or malformed, for example when the downloader failed before writing it.
func readDownloaderResult(path string) *downloaderResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var result downloaderResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return &result
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
