// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux
// +build !linux

package app

import "github.com/spf13/cobra"

// EventsCmd is a command to interact with process lifecycle events. It's currently available only on Linux
var EventsCmd = &cobra.Command{}
