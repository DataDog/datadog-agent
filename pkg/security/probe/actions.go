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
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// KillActionReport defines a kill action reports
type KillActionReport struct {
	sync.RWMutex

	Signal     string
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
