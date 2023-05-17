// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build tools

package proto

// These imports are used to track test and build protobuf tool dependencies.
// This is the currently recommended approach: https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

// NOTE: This package might become obsolete once we can unpin some of the dependencies here listed.
//       At that point it could be wise to merge these back to internal/tools.
//       Though a protobuf dependency, protodep is tracked in internal/tools due to versioning
//       conflicts with the pins set here.

import (
	_ "github.com/golang/mock/mockgen"
	_ "github.com/golang/protobuf/protoc-gen-go"
	_ "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway"
	_ "github.com/tinylib/msgp"
	_ "google.golang.org/grpc"
)
