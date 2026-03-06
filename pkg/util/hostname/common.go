// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostname provides utilities to detect the hostname of the host.
// Deprecated: prefer comp/core/hostname/impl or the comp/core/hostname component.
package hostname

import (
	hostnamedef "github.com/DataDog/datadog-agent/comp/core/hostname/def"
)

// Data contains hostname and the hostname provider.
// Deprecated: use hostnamedef.Data from comp/core/hostname/def.
type Data = hostnamedef.Data
