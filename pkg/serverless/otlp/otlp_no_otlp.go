// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build !otlp

//nolint:revive // TODO(SERV) Fix revive linter
package otlp

import "github.com/DataDog/datadog-agent/pkg/serializer"

//nolint:revive // TODO(SERV) Fix revive linter
type ServerlessOTLPAgent struct{}

//nolint:revive // TODO(SERV) Fix revive linter
func NewServerlessOTLPAgent(serializer.MetricSerializer) *ServerlessOTLPAgent {
	return nil
}

//nolint:revive // TODO(SERV) Fix revive linter
func (o *ServerlessOTLPAgent) Start() {}

//nolint:revive // TODO(SERV) Fix revive linter
func (o *ServerlessOTLPAgent) Stop() {}

//nolint:revive // TODO(SERV) Fix revive linter
func IsEnabled() bool { return false }
