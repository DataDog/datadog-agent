//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// KillActionReport defines a kill action reports
type KillActionReport struct {
	sync.RWMutex

	Signal     string
	Scope      string
	Pid        uint32
	CreatedAt  time.Time
	DetectedAt time.Time
	KilledAt   time.Time
	ExitedAt   time.Time

	// internal
	resolved bool
}

// JKillActionReport used to serialize date
// easyjson:json
type JKillActionReport struct {
	Name       string              `json:"type"`
	Signal     string              `json:"signal"`
	Scope      string              `json:"scope"`
	CreatedAt  utils.EasyjsonTime  `json:"created_at"`
	DetectedAt utils.EasyjsonTime  `json:"detected_at"`
	KilledAt   utils.EasyjsonTime  `json:"killed_at"`
	ExitedAt   *utils.EasyjsonTime `json:"exited_at,omitempty"`
	TTR        string              `json:"ttr,omitempty"`
}

// ToJSON marshal the action
func (k *KillActionReport) ToJSON() ([]byte, error) {
	k.RLock()
	defer k.RUnlock()

	// for sigkill wait for exit
	if k.Signal == "SIGKILL" && !k.resolved {
		return nil, errors.New("not resolved")
	}

	jk := JKillActionReport{
		Name:       rules.KillAction,
		Signal:     k.Signal,
		Scope:      k.Scope,
		CreatedAt:  utils.NewEasyjsonTime(k.CreatedAt),
		DetectedAt: utils.NewEasyjsonTime(k.DetectedAt),
		KilledAt:   utils.NewEasyjsonTime(k.KilledAt),
		ExitedAt:   utils.NewEasyjsonTimeIfNotZero(k.ExitedAt),
	}

	if !k.ExitedAt.IsZero() {
		jk.TTR = k.ExitedAt.Sub(k.CreatedAt).String()
	}

	return utils.MarshalEasyJSON(jk)
}

// Type returns the type of the action report
func (k *KillActionReport) Type() string {
	k.RLock()
	defer k.RUnlock()
	return fmt.Sprintf("%s_%s", rules.KillAction, k.Scope)
}
