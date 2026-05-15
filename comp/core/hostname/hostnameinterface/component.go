// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostnameinterface describes the interface for hostname methods
//
// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def instead.
package hostnameinterface

import (
	hostnameinterfacedef "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
)

// team: agent-runtimes

// Data contains hostname and the hostname provider
//
// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def.Data instead.
type Data = hostnameinterfacedef.Data

// Component is the type for hostname methods.
//
// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def.Component instead.
type Component = hostnameinterfacedef.Component
