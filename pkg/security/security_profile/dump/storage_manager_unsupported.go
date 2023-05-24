// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package dump

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
)

// ActivityDumpStorageManager is defined for unsupported platforms
type ActivityDumpStorageManager struct{}

// PersistRaw is defined for unsupported platforms
func (manager *ActivityDumpStorageManager) PersistRaw(requests []config.StorageRequest, ad *ActivityDump, raw *bytes.Buffer) error {
	return nil
}

// SendTelemetry send telemetry of all storages
func (manager *ActivityDumpStorageManager) SendTelemetry() {}

// NewSecurityAgentStorageManager returns a new instance of ActivityDumpStorageManager
func NewSecurityAgentStorageManager() (*ActivityDumpStorageManager, error) {
	return nil, fmt.Errorf("the activity dump manager is unsupported on this platform")
}

// ActivityDump is defined for unsupported platforms
type ActivityDump struct {
	StorageRequests map[config.StorageFormat][]config.StorageRequest
}

func (ad *ActivityDump) GetImageNameTag() (string, string) {
	return "", ""
}

// NewActivityDumpFromMessage returns a new ActivityDump from a SecurityActivityDumpMessage
func NewActivityDumpFromMessage(msg *api.ActivityDumpMessage) (*ActivityDump, error) {
	return nil, fmt.Errorf("activity dumps are unsupported on this platform")
}
