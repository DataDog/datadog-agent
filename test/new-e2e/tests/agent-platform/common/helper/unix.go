// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package helper implement interfaces to get some information that can be OS specific
package helper

import "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"

// Unix implement helper function for Unix distributions
type Unix struct{}

var _ common.Helper = (*Unix)(nil)

// NewUnixHelper create a new instance of Unix helper
func NewUnixHelper() *Unix { return &Unix{} }

// GetInstallFolder return the install folder path
func (u *Unix) GetInstallFolder() string { return "/opt/datadog-agent/" }

// GetConfigFolder return the config folder path
func (u *Unix) GetConfigFolder() string { return "/etc/datadog-agent/" }

// GetBinaryPath return the datadog-agent binary path
func (u *Unix) GetBinaryPath() string { return "/usr/bin/datadog-agent" }

// GetConfigFileName return the config file name
func (u *Unix) GetConfigFileName() string { return "datadog.yaml" }

// GetServiceName return the service name
func (u *Unix) GetServiceName() string { return "datadog-agent" }

// UnixDogstatsd implement helper function for Dogstatsd on Unix distributions
type UnixDogstatsd struct{}

var _ common.Helper = (*UnixDogstatsd)(nil)

// NewUnixDogstatsdHelper create a new instance of Unix helper for dogstatsd
func NewUnixDogstatsdHelper() *UnixDogstatsd { return &UnixDogstatsd{} }

// GetInstallFolder return the install folder path
func (u *UnixDogstatsd) GetInstallFolder() string { return "/opt/datadog-dogstatsd/" }

// GetConfigFolder return the config folder path
func (u *UnixDogstatsd) GetConfigFolder() string { return "/etc/datadog-dogstatsd/" }

// GetBinaryPath return the datadog-agent binary path
func (u *UnixDogstatsd) GetBinaryPath() string { return "/usr/bin/datadog-dogstatsd" }

// GetConfigFileName return the config file name
func (u *UnixDogstatsd) GetConfigFileName() string { return "dogstatsd.yaml" }

// GetServiceName return the service name
func (u *UnixDogstatsd) GetServiceName() string { return "datadog-dogstatsd" }
