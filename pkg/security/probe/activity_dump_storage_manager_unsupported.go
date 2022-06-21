// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux
// +build !linux

package probe

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/probe/dump"
)

// ActivityDumpStorageManager is defined for unsupported platforms
type ActivityDumpStorageManager struct{}

// PersistRaw is defined for unsupported platforms
func (manager *ActivityDumpStorageManager) PersistRaw(requests []dump.StorageRequest, ad *ActivityDump, raw *bytes.Buffer) error {
	return nil
}

// NewSecurityAgentStorageManager returns a new instance of ActivityDumpStorageManager
func NewSecurityAgentStorageManager() (*ActivityDumpStorageManager, error) {
	return nil, fmt.Errorf("the activity dump manager is unsupported on this platform")
}

// ActivityDump is defined for unsupported platforms
type ActivityDump struct {
	StorageRequests map[dump.StorageFormat][]dump.StorageRequest
}

// NewActivityDumpFromMessage returns a new ActivityDump from a SecurityActivityDumpMessage
func NewActivityDumpFromMessage(msg *api.ActivityDumpMessage) (*ActivityDump, error) {
	return nil, fmt.Errorf("activity dumps are unsupported on this platform")
}
