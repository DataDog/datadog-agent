// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostnameinterface describes the interface for hostname methods.
// Deprecated: import comp/core/hostname/def instead.
package hostnameinterface

import (
	hostnamedef "github.com/DataDog/datadog-agent/comp/core/hostname/def"
)

// team: agent-runtimes

// Data contains hostname and the hostname provider.
// Deprecated: use hostnamedef.Data from comp/core/hostname/def.
type Data = hostnamedef.Data

// Component is the component type for hostname methods.
// Deprecated: use hostnamedef.Component from comp/core/hostname/def.
type Component = hostnamedef.Component
