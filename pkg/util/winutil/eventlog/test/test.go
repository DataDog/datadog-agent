// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows
// +build windows

package eventlog_test

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"

	"github.com/stretchr/testify/require"
)

var enabledAPIsFlag = flag.String("evtapi", "Fake", "Comma-separated list of Event Log APIs to run tests with")

type APITester interface {
	Name() string
	T() testing.TB
	API() evtapi.API
	InstallChannel(channel string) error
	RemoveChannel(channel string) error
	InstallSource(channel string, source string) error
	RemoveSource(channel string, name string) error
	GenerateEvents(channelName string, numEvents uint) error
}

func GetEnabledAPITesters() []string {
	return strings.Split(*enabledAPIsFlag, ",")
}

func GetAPITesterByName(name string, t testing.TB) APITester {
	if name == "Fake" {
		return NewFakeAPITester(t)
	} else if name == "Windows" {
		return NewWindowsAPITester(t)
	}

	require.FailNow(t, fmt.Sprintf("invalid test interface: %v", name))
	return nil
}
