// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// fingerprintSkipReason tells apart the two ways a fingerprint can end up unusable.
type fingerprintSkipReason string

const (
	// fingerprintSkipInsufficientData means the file does not hold enough data to fingerprint yet,
	// as happens right after a rotation. It resolves on its own once the application writes more.
	fingerprintSkipInsufficientData fingerprintSkipReason = "insufficient_data"
	// fingerprintSkipError means computing the fingerprint failed for any reason. eg permissions error
	fingerprintSkipError fingerprintSkipReason = "error"
)

// fingerprintSkipOutcome picks the closing log line. The two outcomes mean opposite things, so they
// cannot share one message: the file either started being tailed, or was never tailed at all.
type fingerprintSkipOutcome string

const (
	// fingerprintSkipRecovered means a tailer was eventually started for the file.
	fingerprintSkipRecovered fingerprintSkipOutcome = "recovered"
	// fingerprintSkipAbandoned means the file stopped being expected, by deletion or by its source
	// being removed, without ever being tailed.
	fingerprintSkipAbandoned fingerprintSkipOutcome = "abandoned"
)

// fingerprintSkip records a file that got no tailer because its fingerprint was unusable. The
// launcher retries every scan, so this exists to report the transitions, not the steady state.
type fingerprintSkip struct {
	// file is refreshed on every check so the paths that end the skip can name it and clear its
	// status message. See recordFingerprintSkip.
	file   *tailer.File
	reason fingerprintSkipReason
	// since is when we first declined to tail the file, for the duration on the closing line.
	since time.Time
	// warned latches the warning so a file that stays unfingerprintable is not reported every scan.
	// It is keyed by reason rather than a single flag: a new reason is worth a fresh warning because
	// the remediation differs, but a file alternating between the two must still warn once per
	// reason rather than once per change.
	warned map[fingerprintSkipReason]bool
}

// recordFingerprintSkip reports that file got no tailer because its fingerprint could not be used,
// which means its logs are being lost. This runs on every scan and on every source addition, so the
// warning is latched here and closed by closeFingerprintSkip.
func (s *Launcher) recordFingerprintSkip(file *tailer.File, fingerprint *types.Fingerprint, err error) {
	// A nil error means the file is only too short for now; anything else is a failure to read it.
	reason := fingerprintSkipInsufficientData
	if err != nil {
		reason = fingerprintSkipError
	}

	scanKey := file.GetScanKey()
	skip, isSkipped := s.fingerprintSkips[scanKey]
	if !isSkipped {
		// Set here as well as below so the field is never nil once stored: the paths that end the
		// skip report skip.file.Path unguarded.
		skip = &fingerprintSkip{
			file:   file,
			reason: reason,
			since:  time.Now(),
			warned: make(map[fingerprintSkipReason]bool, 1),
		}
		s.fingerprintSkips[scanKey] = skip
	} else if skip.reason != reason {
		// Still untailed, only failing differently, so keep the start time: resetting it would
		// report one long gap as two shorter ones.
		skip.reason = reason
	}

	// Adopt the File from this check rather than the first one: a source removed and re-added for
	// the same path has to end up carrying the message. Hand the message over rather than copy it,
	// because this is the last point that still knows about the source losing the file.
	if previous := skip.file; fingerprintSkipMessages(previous) != fingerprintSkipMessages(file) {
		removeFingerprintSkipMessage(previous)
	}
	skip.file = file
	setFingerprintSkipMessage(file, reason, fingerprint, err)

	if skip.warned[reason] {
		return
	}
	skip.warned[reason] = true

	// Remediation lives on the status page rather than here, so the log line stays scannable.
	if reason == fingerprintSkipError {
		log.Warnf("Unable to tail %s. Its fingerprint could not be computed: %v. Logs are not collected until this is resolved.",
			file.Path, err)
		return
	}

	log.Warnf("Unable to tail %s. %s is too short for fingerprinting (needs %s). Logs are not collected until the file grows.",
		file.Path, fingerprintFileSize(file.Path, fingerprint), fingerprintRequirement(fingerprint))
}

// resolveFingerprintSkip stops tracking file as skipped, now that a tailer has been started for it.
func (s *Launcher) resolveFingerprintSkip(file *tailer.File) {
	scanKey := file.GetScanKey()
	if skip, isSkipped := s.fingerprintSkips[scanKey]; isSkipped {
		s.closeFingerprintSkip(scanKey, skip, fingerprintSkipRecovered)
	}
}

// forgetVanishedFiles drops the state of files that are no longer expected to be tailed, so the map
// does not grow with every path that rotates away.
//
// hitFileLimit says the scan result filled every slot, which means it may have left out files that
// are still matched, so it cannot be read as the full set of what is expected. When the limit was
// hit, a skipped file whose source is still active and still on disk may simply have been left out
// of this scan rather than dropped; defer abandonment until one of those stops being true. Source
// removal must end the skip even if the file remains, or stale state survives re-adds and the map
// grows without bound under churning dynamic sources.
func (s *Launcher) forgetVanishedFiles(expected map[string]bool, hitFileLimit bool) {
	for scanKey, skip := range s.fingerprintSkips {
		if expected[scanKey] {
			continue
		}
		if hitFileLimit && fileStillExists(skip.file.Path) && skipSourceStillActive(s.activeSources, skip) {
			continue
		}
		// Last chance to report the gap: we are about to lose track of these files.
		s.closeFingerprintSkip(scanKey, skip, fingerprintSkipAbandoned)
	}
}

// skipSourceStillActive reports whether the source that matched skip.file is still configured to
// be tailed. Pointer identity matches removeSource.
func skipSourceStillActive(activeSources []*sources.LogSource, skip *fingerprintSkip) bool {
	if skip.file == nil || skip.file.Source == nil {
		return false
	}
	source := skip.file.Source.UnderlyingSource()
	if source == nil {
		return false
	}
	for _, active := range activeSources {
		if active == source {
			return true
		}
	}
	return false
}

// fileStillExists reports whether path is still on disk. Anything other than a missing file counts
// as still there, since a file we cannot stat is one to keep retrying rather than give up on.
func fileStillExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// closeFingerprintSkip stops tracking a skipped file, however that came about. Routing both outcomes
// through one function keeps them from drifting apart in what they clean up.
func (s *Launcher) closeFingerprintSkip(scanKey string, skip *fingerprintSkip, outcome fingerprintSkipOutcome) {
	delete(s.fingerprintSkips, scanKey)
	removeFingerprintSkipMessage(skip.file)

	// Only files we warned about get a closing line, so neither half of a gap appears on its own.
	if len(skip.warned) == 0 {
		return
	}
	waited := time.Since(skip.since).Truncate(time.Second)
	if outcome == fingerprintSkipRecovered {
		log.Infof("Now tailing %s, %v after it was first skipped for an unusable fingerprint (%s).",
			skip.file.Path, waited, skip.reason)
		return
	}
	log.Warnf("Stopped tracking %s, never tailed for %v because of an unusable fingerprint (%s). Logs written during that gap were not collected.",
		skip.file.Path, waited, skip.reason)
}

// setFingerprintSkipMessage puts the reason on the status page of the source that matched the file,
// next to the "N files tailed out of M files matching" message that reports the shortfall without
// explaining it. Customers cannot query the Agent's telemetry, and status is where they look first.
func setFingerprintSkipMessage(file *tailer.File, reason fingerprintSkipReason, fingerprint *types.Fingerprint, err error) {
	messages := fingerprintSkipMessages(file)
	if messages == nil {
		return
	}

	if reason == fingerprintSkipError {
		messages.AddMessage(fingerprintSkipMessageKey(file),
			fmt.Sprintf("Not tailing %s: its fingerprint could not be computed (%v)", file.Path, err))
		return
	}

	remediation := ""
	if setting := fingerprintCountSetting(fingerprint); setting != "" {
		remediation = fmt.Sprintf(". Lower %s if this persists", setting)
	}
	messages.AddMessage(fingerprintSkipMessageKey(file),
		fmt.Sprintf("Not tailing %s: too short to fingerprint (needs %s)%s",
			file.Path, fingerprintRequirement(fingerprint), remediation))
}

// fingerprintCountSetting names the count setting behind the threshold, or "" when no setting owns
// it. A per-source fingerprint_config takes precedence over the global one, so naming logs_config
// for those files would point at a setting that cannot change the outcome.
func fingerprintCountSetting(fingerprint *types.Fingerprint) string {
	if fingerprint == nil || fingerprint.Config == nil {
		return ""
	}
	switch fingerprint.Config.Source {
	case types.FingerprintConfigSourcePerSource:
		return "this source's fingerprint_config.count"
	case types.FingerprintConfigSourceDefault:
		return ""
	default:
		return "logs_config.fingerprint_config.count"
	}
}

// removeFingerprintSkipMessage clears the status message once the file stops being skipped, whether
// it ended up being tailed or went away.
func removeFingerprintSkipMessage(file *tailer.File) {
	if messages := fingerprintSkipMessages(file); messages != nil {
		messages.RemoveMessage(fingerprintSkipMessageKey(file))
	}
}

// fingerprintSkipMessages returns the message set of the source that matched file, or nil when there
// is no source to report against.
func fingerprintSkipMessages(file *tailer.File) *config.Messages {
	if file == nil || file.Source == nil {
		return nil
	}
	source := file.Source.UnderlyingSource()
	if source == nil {
		return nil
	}
	return source.Messages
}

// fingerprintSkipMessageKey keys the message per file rather than per source, so a source matching
// several files reports each of them.
func fingerprintSkipMessageKey(file *tailer.File) string {
	return "fingerprintSkip:" + file.GetScanKey()
}

// fingerprintFileSize renders what the file holds, as the subject of a sentence so the message still
// reads when stat fails. Only byte_checksum gets a number: stat cannot count lines, and a size in
// bytes against a threshold in lines would compare two different units.
func fingerprintFileSize(path string, fingerprint *types.Fingerprint) string {
	if fingerprint == nil || fingerprint.Config == nil ||
		fingerprint.Config.FingerprintStrategy != types.FingerprintStrategyByteChecksum {
		return "The file"
	}
	info, err := os.Stat(path)
	if err != nil {
		return "The file"
	}
	return fmt.Sprintf("%d bytes", info.Size())
}

// fingerprintRequirement renders the threshold, in the unit the strategy measures. count is read
// only after seeking past count_to_skip, so when that is set the message says where the count starts
// rather than letting the number read as a total the file already exceeds.
func fingerprintRequirement(fingerprint *types.Fingerprint) string {
	if fingerprint == nil || fingerprint.Config == nil {
		return "more data"
	}
	unit := "lines"
	if fingerprint.Config.FingerprintStrategy == types.FingerprintStrategyByteChecksum {
		unit = "bytes"
	}
	if fingerprint.Config.CountToSkip > 0 {
		return fmt.Sprintf("%d %s after the first %d", fingerprint.Config.Count, unit, fingerprint.Config.CountToSkip)
	}
	return fmt.Sprintf("%d %s", fingerprint.Config.Count, unit)
}
