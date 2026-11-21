// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package logonduration

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type timestampQueries struct {
	fileVaultEnabled func() (bool, error)
	loginWindowTime  func(fileVaultEnabled bool) (time.Time, error)
	loginTime        func() (time.Time, error)
	desktopReadyTime func() (time.Time, error)
}

func collectLoginTimestamps(q timestampQueries) LoginTimestamps {
	result := LoginTimestamps{}
	if fv, err := q.fileVaultEnabled(); err == nil {
		result.FileVaultEnabled = fv
	} else {
		log.Warnf("logonduration: failed to check FileVault status: %v", err)
	}

	lwt, err := q.loginWindowTime(result.FileVaultEnabled)
	if err != nil && result.FileVaultEnabled {
		fvErr := err
		log.Infof("logonduration: FileVault unlock timestamp unavailable, falling back to login window application start: %v", fvErr)
		lwt, err = q.loginWindowTime(false)
		if err != nil {
			err = fmt.Errorf("FileVault unlock: %v; login window application start: %w", fvErr, err)
		}
	}
	if err == nil {
		result.LoginWindowTime = lwt
	} else {
		log.Warnf("logonduration: failed to get login window time: %v", err)
	}

	if lt, err := q.loginTime(); err == nil {
		result.LoginTime = lt
	} else {
		log.Warnf("logonduration: failed to get login time: %v", err)
	}

	if drt, err := q.desktopReadyTime(); err == nil {
		result.DesktopReadyTime = drt
	} else {
		log.Warnf("logonduration: failed to get desktop ready time: %v", err)
	}
	return result
}
