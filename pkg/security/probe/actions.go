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
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// KillActionStatus defines the status of a kill action
type KillActionStatus string

const (
	// KillActionStatusPerformed indicates the kill action was performed
	KillActionStatusPerformed KillActionStatus = "performed"
	// KillActionStatusRuleDisarmed indicates the kill action was skipped because the rule was disarmed
	KillActionStatusRuleDisarmed KillActionStatus = "rule_disarmed"
)

// KillActionReport defines a kill action reports
type KillActionReport struct {
	sync.RWMutex

	Signal       string
	Scope        string
	Status       KillActionStatus
	CreatedAt    time.Time
	DetectedAt   time.Time
	KilledAt     time.Time
	ExitedAt     time.Time
	DisarmerType string

	// internal
	Pid      uint32
	resolved bool
	rule     *rules.Rule
}

// JKillActionReport used to serialize date
// easyjson:json
type JKillActionReport struct {
	Type         string              `json:"type"`
	Signal       string              `json:"signal"`
	Scope        string              `json:"scope"`
	Status       string              `json:"status"`
	DisarmerType string              `json:"disarmer_type,omitempty"`
	CreatedAt    utils.EasyjsonTime  `json:"created_at"`
	DetectedAt   utils.EasyjsonTime  `json:"detected_at"`
	KilledAt     *utils.EasyjsonTime `json:"killed_at,omitempty"`
	ExitedAt     *utils.EasyjsonTime `json:"exited_at,omitempty"`
	TTR          string              `json:"ttr,omitempty"`
}

// IsResolved return if the action is resolved
func (k *KillActionReport) IsResolved() bool {
	k.RLock()
	defer k.RUnlock()

	// for sigkill wait for exit
	return k.Signal != "SIGKILL" || k.resolved || k.Status == KillActionStatusRuleDisarmed
}

// ToJSON marshal the action
func (k *KillActionReport) ToJSON() ([]byte, error) {
	k.RLock()
	defer k.RUnlock()

	jk := JKillActionReport{
		Type:         rules.KillAction,
		Signal:       k.Signal,
		Scope:        k.Scope,
		Status:       string(k.Status),
		DisarmerType: k.DisarmerType,
		CreatedAt:    utils.NewEasyjsonTime(k.CreatedAt),
		DetectedAt:   utils.NewEasyjsonTime(k.DetectedAt),
		KilledAt:     utils.NewEasyjsonTimeIfNotZero(k.KilledAt),
		ExitedAt:     utils.NewEasyjsonTimeIfNotZero(k.ExitedAt),
	}

	if !k.ExitedAt.IsZero() {
		jk.TTR = k.ExitedAt.Sub(k.CreatedAt).String()
	}

	data, err := utils.MarshalEasyJSON(jk)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// IsMatchingRule returns true if this action report is targeted at the given rule ID
func (k *KillActionReport) IsMatchingRule(ruleID eval.RuleID) bool {
	k.RLock()
	defer k.RUnlock()

	return k.rule.ID == ruleID
}
