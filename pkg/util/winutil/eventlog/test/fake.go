// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows

package eventlog_test

import (
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/fake"
)

const (
	// FakeAPIName identifies this API in GetAPITesterByName
	FakeAPIName = "Fake"
)

// FakeAPITester uses a limited in-memory re-implementation of the Windows EventLog APIs
// and provides utilities to the test framework that will simulate behavior and not make
// any changes to the host system
type FakeAPITester struct {
	eventlogapi *fakeevtapi.API
}

// NewFakeAPITester constructs a new API tester for a fake Windows Event Log API
func NewFakeAPITester() *FakeAPITester {
	var ti FakeAPITester
	ti.eventlogapi = fakeevtapi.New()
	return &ti
}

// Name returns the name that identifies this tester for use by GetAPITesterByName
func (ti *FakeAPITester) Name() string {
	return FakeAPIName
}

// API returns the fake API
func (ti *FakeAPITester) API() evtapi.API {
	return ti.eventlogapi
}

// InstallChannel adds a new event log
func (ti *FakeAPITester) InstallChannel(channel string) error {
	return ti.eventlogapi.AddEventLog(channel)
}

// RemoveChannel removes an event log
func (ti *FakeAPITester) RemoveChannel(channel string) error {
	return ti.eventlogapi.RemoveEventLog(channel)
}

// InstallSource adds a new source to an event log channel
func (ti *FakeAPITester) InstallSource(channel string, source string) error {
	return ti.eventlogapi.AddEventSource(channel, source)
}

// RemoveSource removes a source from an event log channel
func (ti *FakeAPITester) RemoveSource(channel string, source string) error {
	return ti.eventlogapi.RemoveEventSource(channel, source)
}

// GenerateEvents creates numEvents new event records
func (ti *FakeAPITester) GenerateEvents(channelName string, numEvents uint) error {
	return ti.eventlogapi.GenerateEvents(channelName, numEvents)
}
