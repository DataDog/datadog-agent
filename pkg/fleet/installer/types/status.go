// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/ssi"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

// InstallerStatus contains the installer status
type InstallerStatus struct {
	Version            string                       `json:"version"`
	Packages           *repository.PackageStates    `json:"packages"`
	ApmInjectionStatus ssi.APMInstrumentationStatus `json:"apm_injection_status"`
	RemoteConfigState  []*remoteConfigPackageState  `json:"remote_config_state"`
}

// RemoteConfigState is the response to the daemon status route.
// It is technically a json-encoded protobuf message but importing
// the protos in the installer binary is too heavy.
type RemoteConfigState struct {
	PackageStates []*remoteConfigPackageState `json:"remote_config_state"`
}

type remoteConfigPackageState struct {
	Package                 string                   `json:"package"`
	StableVersion           string                   `json:"stable_version,omitempty"`
	ExperimentVersion       string                   `json:"experiment_version,omitempty"`
	Task                    *remoteConfigPackageTask `json:"task,omitempty"`
	StableConfigVersion     string                   `json:"stable_config_version,omitempty"`
	ExperimentConfigVersion string                   `json:"experiment_config_version,omitempty"`
}

type remoteConfigPackageTask struct {
	ID    string         `json:"id,omitempty"`
	State int32          `json:"state,omitempty"`
	Error *errorWithCode `json:"error,omitempty"`
}

type errorWithCode struct {
	Code    uint64 `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}
