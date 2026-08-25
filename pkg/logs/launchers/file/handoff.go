// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type replacementIntent struct {
	scanKey              string
	pattern              *regexp.Regexp
	info                 *status.InfoRegistry
	detectedAt           time.Time
	replacementRequested bool
	source               *sources.ReplaceableSource

	evidence tailer.RotationEvidence

	fingerprintConfigAtHandoff *types.FingerprintConfig

	consecutiveEstale int
	firstEstaleAt     time.Time
	lastEstaleAt      time.Time
	lastWarnAt        time.Time
	probeStatus       probeStatus
}

type pathHandoff struct {
	path         string
	draining     map[*tailer.Tailer]struct{}
	replacements map[string]*replacementIntent
}

func normalizeHandoffPath(path string) string {
	if path == "" {
		return path
	}
	clean := filepath.Clean(path)
	if abs, err := filepath.Abs(clean); err == nil {
		return abs
	}
	return clean
}

func (s *Launcher) resolveHandoffSettings(file *tailer.File) types.RotationHandoffSettings {
	if file == nil || file.Source == nil || file.Source.Config() == nil {
		return s.globalHandoffSettings
	}
	settings, err := config.PerSourceRotationHandoffSettings(file.Source.Config(), &s.globalHandoffSettings)
	if err != nil {
		log.Warnf("Invalid rotation handoff settings for %q: %v", file.Path, err)
		return s.globalHandoffSettings
	}
	return settings
}

func (s *Launcher) sequentialHandoffEnabled(file *tailer.File) bool {
	return s.effectiveRotationHandoffMode(file) == types.RotationHandoffModeSequential
}

func (s *Launcher) effectiveRotationHandoffMode(file *tailer.File) types.RotationHandoffMode {
	settings := s.resolveHandoffSettings(file)
	if settings.Mode != types.RotationHandoffModeSequential {
		return types.RotationHandoffModeParallel
	}
	if s.forceSequentialHandoffForTest {
		return types.RotationHandoffModeSequential
	}
	if runtime.GOOS != "linux" {
		s.warnNonLinuxSequentialOnce(file.Source)
		return types.RotationHandoffModeParallel
	}
	return types.RotationHandoffModeSequential
}

func (s *Launcher) warnNonLinuxSequentialOnce(source *sources.ReplaceableSource) {
	if source == nil {
		return
	}
	underlying := source.UnderlyingSource()
	s.nonLinuxSequentialWarnMu.Lock()
	defer s.nonLinuxSequentialWarnMu.Unlock()
	key := underlying.Name
	if key == "" {
		key = underlying.Config.Path
	}
	if _, warned := s.nonLinuxSequentialWarned[key]; warned {
		return
	}
	s.nonLinuxSequentialWarned[key] = struct{}{}
	log.Warnf("rotation_handoff_mode sequential is configured for source %q but sequential handoff is only supported on Linux; falling back to parallel", key)
}

func (s *Launcher) getOrCreatePathHandoff(path string) *pathHandoff {
	s.pathHandoffsMu.Lock()
	defer s.pathHandoffsMu.Unlock()
	if s.pathHandoffs == nil {
		s.pathHandoffs = make(map[string]*pathHandoff)
	}
	normalized := normalizeHandoffPath(path)
	handoff, ok := s.pathHandoffs[normalized]
	if !ok {
		handoff = &pathHandoff{
			path:         normalized,
			draining:     make(map[*tailer.Tailer]struct{}),
			replacements: make(map[string]*replacementIntent),
		}
		s.pathHandoffs[normalized] = handoff
	}
	return handoff
}

func (s *Launcher) pathBlockedBySequentialHandoff(normalizedPath string) bool {
	s.pathHandoffsMu.Lock()
	defer s.pathHandoffsMu.Unlock()
	handoff := s.pathHandoffs[normalizedPath]
	if handoff == nil {
		return false
	}
	for t := range handoff.draining {
		if !t.IsReaderClosed() {
			return true
		}
	}
	return false
}

func (s *Launcher) mayOpenPathForTailing(file *tailer.File) bool {
	if file == nil {
		return true
	}
	return !s.pathBlockedBySequentialHandoff(normalizeHandoffPath(file.Path))
}

func (s *Launcher) beginSequentialHandoff(oldTailer *tailer.Tailer, file *tailer.File, evidence tailer.RotationEvidence) {
	settings := s.resolveHandoffSettings(file)
	oldTailer.BeginSequentialDrain(settings.QuietPeriod, settings.MaxDrain)
	s.tailers.Remove(oldTailer)
	s.rotatedTailers = append(s.rotatedTailers, oldTailer)

	handoff := s.getOrCreatePathHandoff(file.Path)
	s.pathHandoffsMu.Lock()
	handoff.draining[oldTailer] = struct{}{}
	intent := &replacementIntent{
		scanKey:              file.GetScanKey(),
		pattern:              oldTailer.GetDetectedPattern(),
		info:                 oldTailer.GetInfo(),
		detectedAt:           time.Now(),
		replacementRequested: true,
		source:               file.Source,
		evidence:             evidence,
		fingerprintConfigAtHandoff: types.CloneFingerprintConfig(
			s.fingerprinter.GetEffectiveConfigForFile(file),
		),
	}
	handoff.replacements[file.GetScanKey()] = intent
	s.pathHandoffsMu.Unlock()
	s.transferActiveProbeState(file.GetScanKey(), intent)
}

func (s *Launcher) getReplacementIntent(scanKey, path string) *replacementIntent {
	s.pathHandoffsMu.Lock()
	defer s.pathHandoffsMu.Unlock()
	handoff := s.pathHandoffs[normalizeHandoffPath(path)]
	if handoff == nil {
		return nil
	}
	return handoff.replacements[scanKey]
}

func (s *Launcher) replacementIntentToOldInfo(intent *replacementIntent) *oldTailerInfo {
	if intent == nil {
		return nil
	}
	return &oldTailerInfo{
		Pattern:      intent.pattern,
		InfoRegistry: intent.info,
	}
}

func (s *Launcher) onSourceRemoved(source *sources.LogSource) {
	if source == nil {
		return
	}
	var clearedScanKeys []string
	s.pathHandoffsMu.Lock()
	for _, handoff := range s.pathHandoffs {
		for scanKey, intent := range handoff.replacements {
			if intent.source != nil && intent.source.UnderlyingSource() == source {
				intent.replacementRequested = false
				handoff.replacements[scanKey] = intent
				clearedScanKeys = append(clearedScanKeys, scanKey)
			}
		}
	}
	s.pathHandoffsMu.Unlock()
	for _, scanKey := range clearedScanKeys {
		s.clearActiveProbeState(scanKey)
	}
}

func (s *Launcher) clearReplacementIntent(scanKey, path string) {
	s.pathHandoffsMu.Lock()
	defer s.pathHandoffsMu.Unlock()
	handoff := s.pathHandoffs[normalizeHandoffPath(path)]
	if handoff == nil {
		return
	}
	intent := handoff.replacements[scanKey]
	if intent == nil {
		return
	}
	intent.replacementRequested = false
	handoff.replacements[scanKey] = intent
}

func (s *Launcher) reattachReplacementIntent(file *tailer.File) {
	if file == nil || file.Source == nil {
		return
	}
	s.pathHandoffsMu.Lock()
	defer s.pathHandoffsMu.Unlock()
	handoff := s.pathHandoffs[normalizeHandoffPath(file.Path)]
	if handoff == nil {
		return
	}
	intent := handoff.replacements[file.GetScanKey()]
	if intent == nil {
		return
	}
	intent.replacementRequested = true
	intent.source = file.Source
	handoff.replacements[file.GetScanKey()] = intent
}

func (s *Launcher) pruneFinishedPathHandoffs() {
	s.pathHandoffsMu.Lock()
	defer s.pathHandoffsMu.Unlock()
	for path, handoff := range s.pathHandoffs {
		for t := range handoff.draining {
			if t.IsReaderClosed() {
				delete(handoff.draining, t)
			}
		}
		hasPendingReplacement := false
		for _, intent := range handoff.replacements {
			if intent.replacementRequested {
				hasPendingReplacement = true
				break
			}
		}
		if len(handoff.draining) == 0 && !hasPendingReplacement {
			delete(s.pathHandoffs, path)
		}
	}
}

// pathHandoffStateForTest exposes handoff state for unit tests.
type pathHandoffStateForTest struct {
	DrainingCount        int
	ReplacementCount     int
	ReplacementRequested bool
}

func (s *Launcher) pathHandoffStateForTest(path string) pathHandoffStateForTest {
	s.pathHandoffsMu.Lock()
	defer s.pathHandoffsMu.Unlock()
	handoff := s.pathHandoffs[normalizeHandoffPath(path)]
	if handoff == nil {
		return pathHandoffStateForTest{}
	}
	requested := false
	for _, intent := range handoff.replacements {
		if intent.replacementRequested {
			requested = true
			break
		}
	}
	return pathHandoffStateForTest{
		DrainingCount:        len(handoff.draining),
		ReplacementCount:     len(handoff.replacements),
		ReplacementRequested: requested,
	}
}
