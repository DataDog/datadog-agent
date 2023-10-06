// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build !otlp

package otlp

import "github.com/DataDog/datadog-agent/pkg/serializer"

type ServerlessOTLPAgent struct{}

func NewServerlessOTLPAgent(serializer.MetricSerializer) *ServerlessOTLPAgent {
	return nil
}

func (o *ServerlessOTLPAgent) Start() {}

func (o *ServerlessOTLPAgent) Stop() {}

func IsEnabled() bool { return false }
