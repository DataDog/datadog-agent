// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package events

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FallbackListener is used for systems where process collections is not yet supported
type FallbackListener struct {
}

// NewListener returns an error for systems where process events collection is not yet supported
func NewListener(handler EventHandler) (*FallbackListener, error) {
	return nil, errors.New("Process event collection is not yet supported on this system")
}

// Run starts the listener
func (*FallbackListener) Run() {
	log.Error("Process event collection is not yet supported on this system")
}

// Stop stops the listener
func (*FallbackListener) Stop() {
	log.Error("Process event collection is not yet supported on this system")
}
