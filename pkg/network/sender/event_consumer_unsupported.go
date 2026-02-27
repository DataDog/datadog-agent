// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

package sender

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
)

// NewDirectSenderConsumer is not supported on non-linux systems
func NewDirectSenderConsumer(_ EventConsumerRegistry, _ log.Component, _ sysprobeconfig.Component) (eventmonitor.EventConsumer, error) {
	return nil, nil
}
