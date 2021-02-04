// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build tools

package tools

// Those imports are used to track tool dependencies.
// This is the currently recommended approach: https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

import (
	_ "github.com/client9/misspell/cmd/misspell"
	_ "github.com/frapposelli/wwhrd"
	_ "github.com/fzipp/gocyclo"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/gordonklaus/ineffassign"
	_ "github.com/goware/modvendor"
	_ "github.com/shuLhan/go-bindata/cmd/go-bindata"
	_ "github.com/stretchr/testify"
	_ "golang.org/x/lint/golint"
	_ "golang.org/x/mobile/bind"
	_ "golang.org/x/mobile/bind/java"
	_ "golang.org/x/mobile/cmd/gobind"
	_ "golang.org/x/mobile/cmd/gomobile"
	_ "golang.org/x/perf/cmd/benchstat"
	_ "golang.org/x/tools/cmd/stringer"
	_ "golang.org/x/tools/go/ast/astutil"
	_ "golang.org/x/tools/go/gcexportdata"
	_ "gotest.tools/gotestsum"
	_ "honnef.co/go/tools/cmd/staticcheck"
	_ "github.com/mailru/easyjson/easyjson"
)
