//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=readonly -no_std_marshalers -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	// HashTriggerTimeout hash triggered because of a timeout
	HashTriggerTimeout = "timeout"
	// HashTriggerProcessExit hash triggered on process exit
	HashTriggerProcessExit = "process_exit"

	// maxRetryForMsgWithHashAction is the maximum number of retries for a hash action
	// the reports will be marked as resolved after MAX 5 sec (so it doesn't matter if this retry period lasts for longer)
	maxRetryForMsgWithHashAction = 10
)

// HashActionReport defines a hash action reports
// easyjson:json
type HashActionReport struct {
	sync.RWMutex

	Type    string `json:"type"`
	Path    string `json:"path"`
	State   string `json:"state"`
	Trigger string `json:"trigger"`

	// internal
	resolved    bool
	rule        *rules.Rule
	pid         uint32
	seenAt      time.Time
	fileEvent   model.FileEvent
	cgroupID    containerutils.CGroupID
	eventType   model.EventType
	maxFileSize int64
}

// IsResolved return if the action is resolved
func (k *HashActionReport) IsResolved() error {
	k.RLock()
	defer k.RUnlock()

	if k.resolved {
		return nil
	}

	return fmt.Errorf("hash action current state: %+v", k)
}

// MaxRetry implements the DelayabledEvent interface for hash actions
func (k *HashActionReport) MaxRetry() int {
	return maxRetryForMsgWithHashAction
}

// ToJSON marshal the action
func (k *HashActionReport) ToJSON() ([]byte, error) {
	k.Lock()
	defer k.Unlock()

	k.Type = rules.HashAction
	k.Path = k.fileEvent.PathnameStr
	k.State = k.fileEvent.HashState.String()

	data, err := utils.MarshalEasyJSON(k)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// IsMatchingRule returns true if this action report is targeted at the given rule ID
func (k *HashActionReport) IsMatchingRule(ruleID eval.RuleID) bool {
	k.RLock()
	defer k.RUnlock()

	return k.rule.ID == ruleID
}

// PatchEvent implements the EventSerializerPatcher interface
func (k *HashActionReport) PatchEvent(ev *serializers.EventSerializer) {
	if ev.FileEventSerializer == nil {
		return
	}

	ev.FileEventSerializer.HashState = k.fileEvent.HashState.String()
	ev.FileEventSerializer.Hashes = k.fileEvent.Hashes
}

// RawPacketActionReport defines a raw packet action reports
// easyjson:json
type RawPacketActionReport struct {
	sync.RWMutex

	Filter string                `json:"filter"`
	Policy string                `json:"policy"`
	Status RawPacketActionStatus `json:"status"`

	// internal
	rule *rules.Rule
}

type RawPacketActionStatus string

const (
	RawPacketActionStatusPerformed RawPacketActionStatus = "performed"
	RawPacketActionStatusError     RawPacketActionStatus = "error"
)

// IsResolved return if the action is resolved
func (k *RawPacketActionReport) IsResolved() error {
	return nil
}

// MaxRetry implements the DelayabledEvent interface for raw packet actions
func (k *RawPacketActionReport) MaxRetry() int {
	return 0
}

// ToJSON marshal the action
func (k *RawPacketActionReport) ToJSON() ([]byte, error) {
	k.Lock()
	defer k.Unlock()

	data, err := utils.MarshalEasyJSON(k)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// IsMatchingRule returns true if this action report is targeted at the given rule ID
func (k *RawPacketActionReport) IsMatchingRule(ruleID eval.RuleID) bool {
	k.RLock()
	defer k.RUnlock()

	return k.rule.ID == ruleID
}
