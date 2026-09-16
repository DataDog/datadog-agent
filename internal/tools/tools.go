// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build tools

package tools

// Those imports are used to track test and build tool dependencies.
// This is the currently recommended approach: https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

import (
	_ "github.com/aarzilli/whydeadcode"
	_ "github.com/bazelbuild/bazelisk"
	_ "github.com/favadi/protoc-go-inject-tag"
	_ "github.com/frapposelli/wwhrd"
	_ "github.com/go-delve/delve/pkg/goversion"
	_ "github.com/go-enry/go-license-detector/v4/cmd/license-detector"
	_ "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
	_ "github.com/goware/modvendor"
	_ "github.com/mailru/easyjson/easyjson"
	_ "github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto"
	_ "github.com/uber-go/gopatch"
	_ "github.com/vektra/mockery/v3"
	_ "github.com/wadey/gocovmerge"
	_ "golang.org/x/perf/cmd/benchstat"
	_ "golang.org/x/tools/cmd/stringer"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "gotest.tools/gotestsum"
)
