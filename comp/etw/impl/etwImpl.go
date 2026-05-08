// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package etwimpl provides the implementation of the ETW (Event Tracing for Windows) component.
package etwimpl

import "C"
import (
	compdef "github.com/DataDog/datadog-agent/comp/def"
	etw "github.com/DataDog/datadog-agent/comp/etw/def"
)

// Requires defines the dependencies for the etw component.
type Requires struct {
	compdef.In
}

// Provides defines the output of the etw component.
type Provides struct {
	compdef.Out
	Comp etw.Component
}

// NewComponent creates a new etw component.
func NewComponent(_ Requires) (Provides, error) {
	comp, err := NewEtw()
	if err != nil {
		return Provides{}, err
	}
	return Provides{Comp: comp}, nil
}

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
