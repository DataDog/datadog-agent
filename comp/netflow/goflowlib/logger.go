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

func (g *GoflowLoggerAdapter) Printf(format string, params ...interface{}) {
	g.Infof(format, params...)
}

func (g *GoflowLoggerAdapter) Errorf(format string, params ...interface{}) {
	g.Component.Errorf(format, params...)
}

func (g *GoflowLoggerAdapter) Error(params ...interface{}) {
	g.Component.Error(params...)
}

func (g *GoflowLoggerAdapter) Warnf(format string, params ...interface{}) {
	g.Component.Warnf(format, params...)
}

func (g *GoflowLoggerAdapter) Warn(params ...interface{}) {
	g.Component.Warn(params...)
}

func (g *GoflowLoggerAdapter) Fatalf(format string, params ...interface{}) {
	g.Criticalf(format, params...)
}
