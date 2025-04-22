// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"github.com/DataDog/datadog-agent/comp/core/log/def"
)

// GoflowLoggerAdapter is used to implement goflow's logging interface from our logger
// https://github.com/netsampler/goflow2/blob/v1/utils/utils.go#L41-L51
type GoflowLoggerAdapter struct {
	log.Component
}

// Printf logs the given formatted arguments at the info level
func (g *GoflowLoggerAdapter) Printf(format string, params ...interface{}) {
	g.Infof(format, params...)
}

// Errorf logs the given formatted arguments at the error level
func (g *GoflowLoggerAdapter) Errorf(format string, params ...interface{}) {
	g.Component.Errorf(format, params...)
}

// Error logs the given arguments, separated by spaces, at the error level
func (g *GoflowLoggerAdapter) Error(params ...interface{}) {
	g.Component.Error(params...)
}

// Warnf logs the given formatted arguments at the warn level
func (g *GoflowLoggerAdapter) Warnf(format string, params ...interface{}) {
	g.Component.Warnf(format, params...)
}

// Warn logs the given arguments, separated by spaces, at the warn level
func (g *GoflowLoggerAdapter) Warn(params ...interface{}) {
	g.Component.Warn(params...)
}

// Fatalf logs the given formatted arguments at the critical level
func (g *GoflowLoggerAdapter) Fatalf(format string, params ...interface{}) {
	g.Criticalf(format, params...)
}
