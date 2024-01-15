// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows

// Package eventlog_test provides helpers for testing code that uses the eventlog package
package eventlog_test

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"

	"github.com/stretchr/testify/require"
)

// Add command line flag for tests to specify which APIs to use for the tests.
// The default is just "Fake" so that the host is not modified by the tests.
// Specify "Windows" to use the real Event Log APIs.
var enabledAPIsFlag = flag.String("evtapi", FakeAPIName, "Comma-separated list of Event Log APIs to run tests with")

// APITester defines the interface used to test Windows Event Log API implementations.
// Notably it has helpers for adding/removing event logs and sources which would normally
// be the work of an installer.
type APITester interface {
	Name() string
	API() evtapi.API
	InstallChannel(channel string) error
	RemoveChannel(channel string) error
	InstallSource(channel string, source string) error
	RemoveSource(channel string, name string) error
	GenerateEvents(channelName string, numEvents uint) error
}

// GetEnabledAPITesters returns the APIs that are available to test as specified by enabledAPIsFlag
func GetEnabledAPITesters() []string {
	return strings.Split(*enabledAPIsFlag, ",")
}

// GetAPITesterByName returns a new APITester interface with the backing type identified by name.
func GetAPITesterByName(name string, t testing.TB) APITester {
	if name == FakeAPIName {
		return NewFakeAPITester()
	} else if name == WindowsAPIName {
		return NewWindowsAPITester()
	}

	require.FailNow(t, fmt.Sprintf("invalid test interface: %v", name))
	return nil
}
