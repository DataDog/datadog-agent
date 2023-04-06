// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows
// +build windows

package eventlog_test

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/fake"
)

// FakeAPITester uses a limited in-memory re-implementation of the Windows EventLog APIs
// and provides utilities to the test framework that will simulate behavior and not make
// any changes to the host system
type FakeAPITester struct {
	t           testing.TB
	eventlogapi *fakeevtapi.API
}

func NewFakeAPITester(t testing.TB) *FakeAPITester {
	var ti FakeAPITester
	ti.t = t
	ti.eventlogapi = fakeevtapi.New()
	return &ti
}

func (ti *FakeAPITester) Name() string {
	return "Fake"
}

func (ti *FakeAPITester) T() testing.TB {
	return ti.t
}

func (ti *FakeAPITester) API() evtapi.API {
	return ti.eventlogapi
}

func (ti *FakeAPITester) InstallChannel(channel string) error {
	return ti.eventlogapi.AddEventLog(channel)
}

func (ti *FakeAPITester) RemoveChannel(channel string) error {
	return ti.eventlogapi.RemoveEventLog(channel)
}

func (ti *FakeAPITester) InstallSource(channel string, source string) error {
	return ti.eventlogapi.AddEventSource(channel, source)
}

func (ti *FakeAPITester) RemoveSource(channel string, source string) error {
	return ti.eventlogapi.RemoveEventSource(channel, source)
}

func (ti *FakeAPITester) GenerateEvents(channelName string, numEvents uint) error {
	return ti.eventlogapi.GenerateEvents(channelName, numEvents)
}
