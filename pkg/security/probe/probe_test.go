// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	smodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

type MockHandlerP1 struct{}

func (m MockHandlerP1) Priority() int                 { return 1 }
func (m MockHandlerP1) ResolveEvent(_ *smodel.Event)  {}
func (m MockHandlerP1) HandleEvent(_ *smodel.ROEvent) {}

type MockHandlerP2 struct{}

func (m MockHandlerP2) Priority() int                 { return 2 }
func (m MockHandlerP2) ResolveEvent(_ *smodel.Event)  {}
func (m MockHandlerP2) HandleEvent(_ *smodel.ROEvent) {}

type MockHandlerP3 struct{}

func (m MockHandlerP3) Priority() int                 { return 3 }
func (m MockHandlerP3) ResolveEvent(_ *smodel.Event)  {}
func (m MockHandlerP3) HandleEvent(_ *smodel.ROEvent) {}

func TestAddEventHandler(t *testing.T) {
	var p Probe

	p.AddEventHandler(smodel.ExecEventType, MockHandlerP3{})
	p.AddEventHandler(smodel.ExecEventType, MockHandlerP2{})
	p.AddEventHandler(smodel.ExecEventType, MockHandlerP1{})
	p.AddEventHandler(smodel.ExecEventType, MockHandlerP2{})

	expected := []EventHandler{
		MockHandlerP1{},
		MockHandlerP2{},
		MockHandlerP2{},
		MockHandlerP3{},
	}
	actual := p.eventHandlers[smodel.ExecEventType]

	assert.Equal(t, expected, actual)
}
