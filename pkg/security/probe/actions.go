//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=readonly -no_std_marshalers $GOFILE

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
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// KillActionStatus defines the status of a kill action
type KillActionStatus string

const (
	// KillActionStatusError indicates the kill action failed
	KillActionStatusError KillActionStatus = "error"
	// KillActionStatusPerformed indicates the kill action was performed
	KillActionStatusPerformed KillActionStatus = "performed"
	// KillActionStatusRuleDisarmed indicates the kill action was skipped because the rule was disarmed
	KillActionStatusRuleDisarmed KillActionStatus = "rule_disarmed"
	// KillActionStatusRuleDismantled indicates the kill action was skipped because the rule was dismantled
	KillActionStatusRuleDismantled KillActionStatus = "rule_dismantled"
	// KillActionStatusQueued indicates the kill action was queued until the end of the first rule period
	KillActionStatusQueued KillActionStatus = "kill_queued"
	// KillActionStatusPartiallyPerformed indicates the kill action was performed on some processes but not all
	KillActionStatusPartiallyPerformed = "partially_performed"

	// maxRetryForMsgWithKillAction is the maximum number of retries for a kill action
	// - a kill can be queued up to the end of the first disarmer period (1min by default)
	// - so, we set the server retry period to 1min and 2sec (+2sec to have the time to trigger the kill and wait to catch the process exit)
	maxRetryForMsgWithKillAction = 62
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
func (k *KillActionReport) IsResolved() error {
	k.RLock()
	defer k.RUnlock()

	// for sigkill wait for exit
	if k.Signal != "SIGKILL" || k.resolved || k.Status == KillActionStatusRuleDisarmed {
		return nil
	}
	return fmt.Errorf("kill action current state: %+v", k)
}

// MaxRetry implements the DelayabledEvent interface for kill actions
func (k *KillActionReport) MaxRetry() int {
	return maxRetryForMsgWithKillAction
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
