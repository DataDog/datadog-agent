// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package languagedetection

import (
	"os"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/internal/detectors"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var detectorsWithPrivilege = []languagemodels.Detector{
	detectors.NewGoDetector(),
}

var (
	PermissionDeniedWarningOnce = sync.Once{}
)

func handleDetectorError(err error) {
	if os.IsPermission(err) {
		PermissionDeniedWarningOnce.Do(func() {
			log.Warnf("Attempted to detect language but permission was denied. Make sure the system probe is running as root and has CAP_PTRACE if it is running in a container.")
		})
	}
}

func DetectWithPrivileges(procs []languagemodels.Process) []languagemodels.Language {
	languages := make([]languagemodels.Language, len(procs))
	for i, proc := range procs {
		for _, detector := range detectorsWithPrivilege {
			lang, err := detector.DetectLanguage(proc)
			if err != nil {
				handleDetectorError(err)
				continue
			}
			languages[i] = lang
		}
	}
	return languages
}

func MockPrivilegedDetectors(t *testing.T, newDetectors []languagemodels.Detector) {
	oldDetectors := detectorsWithPrivilege
	t.Cleanup(func() { detectorsWithPrivilege = oldDetectors })
	detectorsWithPrivilege = newDetectors
}
