// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"

// Keep local aliases so the implementation and its tests use the component's
// public wire model without duplicating it.
type Log = agenttelemetry.Log
type LogsPayload = agenttelemetry.LogsPayload

const LogLevelError = agenttelemetry.LogLevelError
