// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package privileged implements language detection that relies on elevated permissions.
//
// An example of privileged language detection would be binary analysis, where the binary must be
// inspected to determine the language it was compiled from.
package privileged

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"

	"github.com/hashicorp/golang-lru/v2/simplelru"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/internal/detectors"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var detectorsWithPrivilege = []languagemodels.Detector{
	detectors.NewGoDetector(),
}

var (
	permissionDeniedWarningOnce = sync.Once{}
)

func handleDetectorError(err error) {
	if os.IsPermission(err) {
		permissionDeniedWarningOnce.Do(func() {
			log.Warnf("Attempted to detect language but permission was denied. Make sure the " +
				"system probe is running as root and has CAP_PTRACE if it is running in a " +
				"container.")
		})
	}
}

// LanguageDetector is a struct that is used by the system probe to run through the list of detectors that require
// elevated privileges to run.
// It contains some extra state such as a cached hostProc value, as well as a cache for processes that reuse a binary
// which has already been seen.
type LanguageDetector struct {
	hostProc      string
	binaryIDCache *simplelru.LRU[binaryID, languagemodels.Language]
	detectors     []languagemodels.Detector
}

// NewLanguageDetector constructs a new LanguageDetector
func NewLanguageDetector() LanguageDetector {
	lru, _ := simplelru.NewLRU[binaryID, languagemodels.Language](1000, nil) // Only errors if the size is negative, so it's safe to ignore

	return LanguageDetector{
		detectors:     detectorsWithPrivilege,
		hostProc:      kernel.ProcFSRoot(),
		binaryIDCache: lru,
	}
}

// DetectWithPrivileges is used by the system probe to detect languages for languages that require binary analysis to detect.
func (l *LanguageDetector) DetectWithPrivileges(procs []languagemodels.Process) []languagemodels.Language {
	languages := make([]languagemodels.Language, len(procs))
	for i, proc := range procs {
		bin, err := l.getBinID(proc)
		if err != nil {
			handleDetectorError(err)
			log.Debug("failed to get binID:", err)
		}

		if lang, ok := l.binaryIDCache.Get(bin); ok {
			log.Tracef("Pid %v already detected as %v, skipping", proc.GetPid(), lang.Name)
			languages[i] = lang
			continue
		}

		var lang languagemodels.Language
		for _, detector := range l.detectors {
			var err error
			lang, err = detector.DetectLanguage(proc)
			if err != nil {
				handleDetectorError(err)
				continue
			}
			languages[i] = lang
		}
		l.binaryIDCache.Add(bin, lang)
	}
	return languages
}

// MockPrivilegedDetectors is used in tests to inject mock tests. It should be called before `DetectWithPrivileges`
func MockPrivilegedDetectors(t *testing.T, newDetectors []languagemodels.Detector) {
	oldDetectors := detectorsWithPrivilege
	t.Cleanup(func() { detectorsWithPrivilege = oldDetectors })
	detectorsWithPrivilege = newDetectors
}

func (l *LanguageDetector) getBinID(process languagemodels.Process) (binaryID, error) {
	procPath := filepath.Join(l.hostProc, strconv.Itoa(int(process.GetPid())))
	exePath := filepath.Join(procPath, "exe")
	binPath, err := os.Readlink(exePath)
	if err != nil {
		return binaryID{}, fmt.Errorf("readlink %s: %v", exePath, err)
	}

	binPath = filepath.Join(procPath, "root", binPath)

	var stat syscall.Stat_t
	err = syscall.Stat(binPath, &stat)
	if err != nil {
		return binaryID{}, fmt.Errorf("stat binary path %s: %v", binPath, err)
	}

	return binaryID{
		dev: stat.Dev,
		ino: stat.Ino,
	}, nil
}

type binaryID struct {
	dev, ino uint64
}
