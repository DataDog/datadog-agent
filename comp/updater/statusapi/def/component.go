// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package statusapi is the installer read-only status api component.
package statusapi

// team: fleet windows-products

// Component is the interface for the installer status api component.
//
// It is empty on purpose: the component's only job is to run the listener for the
// lifetime of the daemon, and exposing Start/Stop would let a second caller start it
// again behind the lifecycle's back.
type Component interface{}
