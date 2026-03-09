// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package agentimpl

import configComponent "github.com/DataDog/datadog-agent/comp/core/config"

// autoProfileWatchdog is not used in serverless builds.
type autoProfileWatchdog struct{}

func (a *logAgent) registerAutoProfileModeCallback(_ configComponent.Component) {}

func (a *logAgent) onStartAutoProfileMode() error { return nil }

func (a *logAgent) onStopAutoProfileMode() {}
