// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build tools

package tools

// Those imports are used to track test and build tool dependencies.
// This is the currently recommended approach: https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

import (
	_ "github.com/frapposelli/wwhrd"
	_ "github.com/go-enry/go-license-detector/v4/cmd/license-detector"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/goware/modvendor"
	_ "github.com/mgechev/revive"
	_ "github.com/stormcat24/protodep"
	_ "github.com/vektra/mockery/v2"
	_ "golang.org/x/mobile/cmd/gobind"
	_ "golang.org/x/mobile/cmd/gomobile"
	_ "golang.org/x/perf/cmd/benchstat"
	_ "gotest.tools/gotestsum"
)
