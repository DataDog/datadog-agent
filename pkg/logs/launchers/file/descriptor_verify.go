// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"io"
	"path/filepath"

	"github.com/spf13/afero"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (s *Launcher) tryStartVerifiedSequentialReplacement(file *tailer.File, oldInfo *oldTailerInfo, intent *replacementIntent) bool {
	if file == nil || intent == nil || oldInfo == nil {
		return false
	}

	scanKey := file.GetScanKey()
	path := file.Path
	effectiveConfig := s.fingerprinter.GetEffectiveConfigForFile(file)
	if !types.FingerprintConfigsEqual(effectiveConfig, intent.fingerprintConfigAtHandoff) {
		log.Warnf("fingerprint config changed during sequential handoff for %q; re-probing pathname", file.Path)
		s.invalidateReplacementEvidence(scanKey, path)
		s.updateReplacementConfigSnapshot(scanKey, path, effectiveConfig)
		intent = s.getReplacementIntent(scanKey, path)
		if intent == nil {
			return false
		}
	}
	if !types.DirectConfigured(effectiveConfig) {
		return s.startNewTailerWithStoredInfoLegacy(file, oldInfo, intent)
	}

	if !intent.evidence.HasAuthoritativeDirectCandidate(effectiveConfig) {
		result, err := s.fingerprinter.ComputeFingerprintResult(file)
		if err != nil {
			if tailer.IsStaleFileHandle(err) {
				s.noteIntentEstale(scanKey, path)
			}
			return false
		}
		if !HasAuthoritativeDirectCandidate(effectiveConfig, result) {
			s.setIntentProbeStatus(scanKey, path, probeStatusBufferedProbeRejected)
			return false
		}
		s.updateIntentEvidence(file, scanKey, path, result)
		intent = s.getReplacementIntent(scanKey, path)
		if intent == nil {
			return false
		}
	}

	s.setIntentProbeStatus(scanKey, path, probeStatusVerifying)

	fullpath, err := filepath.Abs(file.Path)
	if err != nil {
		return false
	}

	fd, err := s.fileOpener.OpenLogFile(fullpath)
	if err != nil {
		if tailer.IsStaleFileHandle(err) {
			s.noteIntentEstale(scanKey, path)
		}
		return false
	}

	candidateConfig := intent.evidence.Fingerprint.Config
	if candidateConfig == nil {
		candidateConfig = effectiveConfig
	}

	fdFingerprint, err := s.fingerprinter.ComputeFingerprintFromHandle(fd, candidateConfig)
	if err != nil {
		fd.Close()
		if tailer.IsStaleFileHandle(err) {
			s.noteIntentEstale(scanKey, path)
		}
		return false
	}

	if _, err := fd.Seek(0, io.SeekStart); err != nil {
		fd.Close()
		return false
	}

	if !types.FingerprintsMatchUnderSameConfig(fdFingerprint, intent.evidence.Fingerprint) {
		fd.Close()
		s.setIntentProbeStatus(scanKey, path, probeStatusDescriptorMismatch)
		log.Warnf("sequential replacement descriptor mismatch for %q: refusing to start tailer", file.Path)
		return false
	}

	return s.startVerifiedTailerWithOpenFile(file, oldInfo, fd, fdFingerprint, intent)
}

func (s *Launcher) updateIntentEvidence(file *tailer.File, scanKey, path string, result tailer.FingerprintResult) {
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
	current := s.fingerprinter.GetEffectiveConfigForFile(file)
	if !types.FingerprintConfigsEqual(current, intent.fingerprintConfigAtHandoff) {
		intent.evidence = tailer.RotationEvidence{}
		handoff.replacements[scanKey] = intent
		return
	}
	intent.evidence.Fingerprint = result.Fingerprint
	intent.evidence.AppliedFlags = append([]types.FileOpenFlag(nil), result.AppliedFlags...)
	intent.probeStatus = probeStatusOK
	handoff.replacements[scanKey] = intent
}

func (s *Launcher) invalidateReplacementEvidence(scanKey, path string) {
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
	intent.evidence = tailer.RotationEvidence{}
	handoff.replacements[scanKey] = intent
}

func (s *Launcher) updateReplacementConfigSnapshot(scanKey, path string, config *types.FingerprintConfig) {
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
	intent.fingerprintConfigAtHandoff = types.CloneFingerprintConfig(config)
	handoff.replacements[scanKey] = intent
}

func (s *Launcher) startVerifiedTailerWithOpenFile(file *tailer.File, oldInfo *oldTailerInfo, fd afero.File, fingerprint *types.Fingerprint, _ *replacementIntent) bool {
	channel, monitor := s.pipelineProvider.NextPipelineChanWithMonitor()

	var tailerInfo *status.InfoRegistry
	if oldInfo.InfoRegistry != nil {
		tailerInfo = oldInfo.InfoRegistry
	} else {
		tailerInfo = status.NewInfoRegistry()
	}

	var decoderInstance decoder.Decoder
	if oldInfo.Pattern != nil {
		decoderInstance = decoder.NewDecoderFromSourceWithPattern(file.Source, oldInfo.Pattern, tailerInfo)
	} else {
		decoderInstance = decoder.NewDecoderFromSource(file.Source, tailerInfo)
	}

	tailerInstance := tailer.NewTailer(&tailer.TailerOptions{
		OutputChan:      channel,
		File:            file,
		SleepDuration:   s.tailerSleepDuration,
		Decoder:         decoderInstance,
		Info:            tailerInfo,
		TagAdder:        s.tagger,
		CapacityMonitor: monitor,
		Registry:        s.registry,
		Fingerprint:     fingerprint,
		Fingerprinter:   s.fingerprinter,
		Rotated:         true,
		FileOpener:      s.fileOpener,
	})
	addFingerprintConfigToTailerInfo(tailerInstance)

	log.Infof("Starting verified sequential replacement tailer for: %s (scan key %s)", file.Path, file.GetScanKey())
	if err := tailerInstance.StartWithOpenFile(fd, 0, io.SeekStart); err != nil {
		log.Warn(err)
		return false
	}

	s.tailers.Add(tailerInstance)
	s.clearReplacementIntent(file.GetScanKey(), file.Path)
	return true
}

func (s *Launcher) startNewTailerWithStoredInfoLegacy(file *tailer.File, oldInfo *oldTailerInfo, _ *replacementIntent) bool {
	var fingerprint *types.Fingerprint
	var err error

	if s.fingerprinter.ShouldFileFingerprint(file) {
		fingerprint, err = s.fingerprinter.ComputeFingerprint(file)
		if (fingerprint != nil && !fingerprint.ValidFingerprint()) || err != nil {
			return false
		}
	} else if fpConfig := s.fingerprinter.GetEffectiveConfigForFile(file); fpConfig != nil {
		fingerprint = &types.Fingerprint{
			Value:  types.InvalidFingerprintValue,
			Config: fpConfig,
		}
	}

	return s.startNewTailerWithStoredInfo(file, config.ForceBeginning, oldInfo, fingerprint)
}

func HasAuthoritativeDirectCandidate(config *types.FingerprintConfig, result tailer.FingerprintResult) bool {
	return tailer.HasAuthoritativeDirectCandidate(config, result)
}
