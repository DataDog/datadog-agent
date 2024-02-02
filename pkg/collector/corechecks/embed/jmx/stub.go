// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !jmx

package jmx

import dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"

// RegisterJMXCheckLoader explicitly registers the JMXCheckLoader and injects the dogstatsd server component
func RegisterJMXCheckLoader(server dogstatsdServer.Component) {}
