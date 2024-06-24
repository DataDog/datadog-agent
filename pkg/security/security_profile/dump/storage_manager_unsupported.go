// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package dump holds dump related files
package dump

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
)

// ActivityDumpStorageManager is defined for unsupported platforms
type ActivityDumpStorageManager struct{}

// PersistRaw is defined for unsupported platforms
func (manager *ActivityDumpStorageManager) PersistRaw(_ []config.StorageRequest, _ *ActivityDump, _ *bytes.Buffer) error {
	return nil
}

// SendTelemetry send telemetry of all storages
func (manager *ActivityDumpStorageManager) SendTelemetry() {}

// NewAgentStorageManager returns a new instance of ActivityDumpStorageManager
func NewAgentStorageManager(_ sender.SenderManager) (*ActivityDumpStorageManager, error) {
	return nil, fmt.Errorf("the activity dump manager is unsupported on this platform")
}

// ActivityDump is defined for unsupported platforms
type ActivityDump struct {
	StorageRequests map[config.StorageFormat][]config.StorageRequest
}

// GetImageNameTag returns the image name and tag of this activity dump
func (ad *ActivityDump) GetImageNameTag() (string, string) {
	return "", ""
}

// NewActivityDumpFromMessage returns a new ActivityDump from a SecurityActivityDumpMessage
func NewActivityDumpFromMessage(_ *api.ActivityDumpMessage) (*ActivityDump, error) {
	return nil, fmt.Errorf("activity dumps are unsupported on this platform")
}
