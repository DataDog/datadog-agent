// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package pythonchecks defines the GUI boundary for listing Python integrations.
package pythonchecks

// Lister lists Python integrations installed as wheels.
type Lister interface {
	GetPythonIntegrationList() ([]string, error)
}
