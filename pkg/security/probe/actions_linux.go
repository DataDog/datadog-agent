//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// HashActionReport defines a hash action reports
// easyjson:json
type HashActionReport struct {
	sync.RWMutex

	Type  string `json:"type"`
	Path  string `json:"path"`
	State string `json:"state"`

	// internal
	resolved  bool
	rule      *rules.Rule
	pid       uint32
	seenAt    time.Time
	fileEvent model.FileEvent
	crtID     containerutils.ContainerID
	eventType model.EventType
}

// IsResolved return if the action is resolved
func (k *HashActionReport) IsResolved() bool {
	k.RLock()
	defer k.RUnlock()

	return k.resolved
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
