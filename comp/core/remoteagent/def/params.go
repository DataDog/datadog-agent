// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remoteagent ... /* TODO: detailed doc comment for the component */
package remoteagent

// team: /* TODO: add team name */

// Params privides the params for the remoteagent component
type Params struct {
	ID                string
	DisplayName       string
	Endpoint          string
	AuthToken         string
	StatusCallback    func() map[string]string
	FlareCallback     func() map[string][]byte
	TelemetryCallback func() string
}
