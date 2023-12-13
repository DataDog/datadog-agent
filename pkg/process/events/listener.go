// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package events

import "github.com/DataDog/datadog-agent/pkg/process/events/model"

// EventHandler is a function used by the listener to handle a collected process event
type EventHandler func(e *model.ProcessEvent)
