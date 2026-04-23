// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the flare component.
package fx

import (
	flare "github.com/DataDog/datadog-agent/comp/core/flare"
	flaredef "github.com/DataDog/datadog-agent/comp/core/flare/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module(params flaredef.Params) fxutil.Module {
	return flare.Module(params)
}
