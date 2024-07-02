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
	fx.Provide(NewEtw),
)

// NewEtw returns a new etw component. It is exported so that it can
// be used by consumers that aren't components themselves.
func NewEtw() (etw.Component, error) {
	return &etwImpl{}, nil
}

type etwImpl struct {
}

func (s *etwImpl) NewSession(sessionName string, f etw.SessionConfigurationFunc) (etw.Session, error) {
	session, err := createEtwSession(sessionName, f)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *etwImpl) NewWellKnownSession(sessionName string, f etw.SessionConfigurationFunc) (etw.Session, error) {
	session, err := createWellKnownEtwSession(sessionName, f)
	if err != nil {
		return nil, err
	}
	return session, nil
}
