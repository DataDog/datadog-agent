// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !darwin && !windows

package runners

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

// RunLocalRemediation reports that rshell is unavailable on this platform.
func (*WorkflowRunner) RunLocalRemediation(context.Context, *pb.RunLocalRemediationRequest) (*pb.RunLocalRemediationResponse, error) {
	return nil, status.Error(codes.Unimplemented, "local remediation is unsupported on this platform")
}
