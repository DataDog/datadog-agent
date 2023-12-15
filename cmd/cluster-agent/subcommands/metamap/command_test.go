// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package metamap implements 'cluster-agent metamap'.
package metamap

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestMetamapCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"metamap", "nodeName"},
		run,
		func() {})
}
