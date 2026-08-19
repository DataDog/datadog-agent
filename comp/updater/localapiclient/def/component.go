// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package localapiclient provides the installer local API client component.
package localapiclient

import localapi "github.com/DataDog/datadog-agent/comp/updater/localapi/def"

// team: fleet windows-products

// StatusClient is the read-only portion of the client used by Agent status.
type StatusClient interface {
	Status() (localapi.StatusResponse, error)
}

// Component is the full local API client used by the installer CLI.
type Component interface {
	StatusClient

	SetCatalog(catalog string) error
	SetConfigCatalog(configs string) error
	Install(pkg, version string) error
	Remove(pkg string) error
	StartExperiment(pkg, version string) error
	StopExperiment(pkg string) error
	PromoteExperiment(pkg string) error
	StartConfigExperiment(pkg, operations string, encryptedSecrets map[string]string) error
	StopConfigExperiment(pkg string) error
	PromoteConfigExperiment(pkg string) error
}
