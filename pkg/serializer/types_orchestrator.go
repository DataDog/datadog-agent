// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package serializer

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/util/api"
)

// ProcessMessageBody is a type alias for processes proto message body
type ProcessMessageBody = model.MessageBody

var processPayloadEncoder = api.EncodePayload
