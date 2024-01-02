// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package etwimpl

import "C"
import (
	"github.com/DataDog/datadog-agent/comp/etw"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newEtw),
)

func newEtw() (etw.Component, error) {
	return &etwImpl{}, nil
}

type etwImpl struct {
}

func (s *etwImpl) NewSession(sessionName string) (etw.Session, error) {
	session, err := createEtwSession(sessionName)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// NewETWSessionWithoutInit is a
//
// temporary implmentation to allow system probe access to this component
// without being a component itself.  Consumer is buried deep in system
// probe, and hasn't been componentized yet.
func NewETWSessionWithoutInit() etw.Component {
	return &etwImpl{}
}
