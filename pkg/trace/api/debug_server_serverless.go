// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build serverless

package api

import "github.com/DataDog/datadog-agent/pkg/trace/config"

type DebugServer struct{}

func NewDebugServer(conf *config.AgentConfig) *DebugServer {
	return new(DebugServer)
}

func (*DebugServer) Start() {}
func (*DebugServer) Stop()  {}
