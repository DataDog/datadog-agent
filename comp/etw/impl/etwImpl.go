// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package etwimpl

import "C"
import (
	"context"
	"github.com/DataDog/datadog-agent/comp/etw"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newEtw),
)

type etwImpl struct {
	sessions []etw.Session
}

func (s *etwImpl) start(_ context.Context) error {
	// Nothing to do
	return nil
}

func (s *etwImpl) stop(_ context.Context) error {
	for _, session := range s.sessions {
		session.StopTracing()
	}
	return nil
}

func (s *etwImpl) NewSession(sessionName string) (etw.Session, error) {
	session, err := CreateEtwSession(sessionName)
	if err != nil {
		return nil, err
	}
	s.sessions = append(s.sessions, session)
	return session, nil
}

type dependencies struct {
	fx.In
	Lc fx.Lifecycle
}

func newEtw(deps dependencies) (etw.Component, error) {
	etw := &etwImpl{}
	deps.Lc.Append(fx.Hook{OnStart: etw.start, OnStop: etw.stop})
	return etw, nil
}
