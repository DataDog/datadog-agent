// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive
package flare

import (
	"sync"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

// FlareController is a type that contains information needed to insert into a
// flare from the logs agent.
type FlareController struct {
	mu       sync.Mutex
	allFiles []string
}

// NewFlareController creates a new FlareController
func NewFlareController() *FlareController {
	panic("not called")
}

// FillFlare is the callback function for the flare where information in the
// FlareController can be printed.
func (fc *FlareController) FillFlare(fb flaretypes.FlareBuilder) error {
	panic("not called")
}

// SetAllFiles assigns the allFiles parameter of FlareController
func (fc *FlareController) SetAllFiles(files []string) {
	panic("not called")
}
