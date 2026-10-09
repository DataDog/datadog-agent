// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package azurefiles tests log collection from Azure Files across rotation,
// through a kernel CIFS mount and through the native SMB log source.
package azurefiles

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	azureresources "github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/azure/aks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/azure/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	azurekubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/azure/kubernetes"
)

const (
	e2eNamespace          = "azure-files-e2e"
	agentNamespace        = "datadog"
	storageSecretPrefix   = "azure-files-storage"
	accountNameSecretKey  = "azurestorageaccountname"
	accountKeySecretKey   = "azurestorageaccountkey"
	agentSecretsMountRoot = "/etc/azure-files-secrets"
	writerContainerName   = "writer"
	ledgerContainerName   = "ledger"
	appenderContainerName = "appender"
	logMountPath          = "/mnt/azure-files"
	activeLogName         = "app.log"
	ledgerName            = "ledger.jsonl"
	// periodsJournalName is the stock writer's journal, which its ledger reads.
	periodsJournalName   = "periods.jsonl"
	writerTargetSequence = "81792,82885,84060,81920,85000,82500"
	// streamDirPrefix names the stream directories of a multi-stream writer,
	// svc-1 to svc-<N>.
	streamDirPrefix  = "svc-"
	partOfLabel      = "app.kubernetes.io/part-of"
	partOfLabelValue = "azure-files-e2e"
	runIDLabel       = "e2e.datadoghq.com/run-id"
	cellLabel        = "e2e.datadoghq.com/cell"
)

// The writer pods run the stock image below unless a run names its own image.
// The stock image has Python and a POSIX shell but no Java, so the writer is
// workload/logwriter.py, a port of the Java writer that keeps its record,
// rotation and checksum formats, and the writer, the ledger and the appender
// come from a ConfigMap mounted where the Java image keeps its own copies.
const (
	// stockWorkloadImage is a Docker Hub image, pulled through the
	// environment's Docker Hub mirror; on Azure that is Docker Hub itself. The
	// digest pins the multi-arch index of the tag.
	stockWorkloadImage    = "library/python:3.12.15-slim-bookworm@sha256:54c85f3c47607a77f32adec749d3c81d1348bf25833671f512b26a9b6d778cb3"
	workloadConfigMapName = "azure-files-workload"
	workloadVolumeName    = "workload"
	workloadDir           = "/app"
	pythonWriterScript    = "logwriter.py"
	ledgerScript          = "ledger.sh"
	appenderScript        = "appender.sh"
	// stockWorkloadUser runs the stock containers. The Java image runs as a
	// user of its own; neither needs root on the CIFS mount.
	stockWorkloadUser = 1000
	// workloadChecksumAnnotation changes the writer pod template whenever the
	// ConfigMap scripts change, so a reused stack restarts the writers.
	workloadChecksumAnnotation = "e2e.datadoghq.com/workload-sha256"
	// storageAccountsConfigMapName maps each cell to its storage account's
	// ARM ID; it holds no key.
	storageAccountsConfigMapName = "azure-files-storage-accounts"
)

var (
	//go:embed workload/logwriter.py
	pythonWriterSource string
	//go:embed workload/ledger.sh
	ledgerSource string
	//go:embed workload/appender.sh
	appenderSource string
)

// workloadKind names where the writer, ledger and appender come from.
type workloadKind string

const (
	// stockWorkload runs the ConfigMap scripts on stockWorkloadImage.
	stockWorkload workloadKind = "python"
	// customWorkload runs the Java writer image built from workload/, which
	// carries its own scripts.
	customWorkload workloadKind = "java"
)

type envVar struct {
	name  string
	value string
}

// workloadRuntime is how the three containers of a writer pod run.
type workloadRuntime struct {
	kind  workloadKind
	image string
	// writerCommand is nil when the image entrypoint runs the writer.
	writerCommand   []string
	ledgerCommand   []string
	appenderCommand []string
	// writerEnv and ledgerEnv are appended to those containers' environment.
	writerEnv []envVar
	ledgerEnv []envVar
	// scripts are the files of the workload ConfigMap, by file name, mounted
	// at workloadDir. They are empty when the image carries the scripts.
	scripts map[string]string
}

// workloadKind is the Java writer image when the run names one, and the stock
// image otherwise.
func (spec runSpec) workloadKind() workloadKind {
	if spec.writerImage != "" {
		return customWorkload
	}
	return stockWorkload
}

func (spec runSpec) workloadRuntime(dockerhubMirror string) workloadRuntime {
	ledgerCommand := []string{"/bin/sh", workloadDir + "/" + ledgerScript}
	appenderCommand := []string{"/bin/sh", workloadDir + "/" + appenderScript}
	if spec.workloadKind() == customWorkload {
		return workloadRuntime{
			kind:            customWorkload,
			image:           spec.writerImage,
			ledgerCommand:   ledgerCommand,
			appenderCommand: appenderCommand,
			writerEnv:       []envVar{{"JAVA_TOOL_OPTIONS", "-Duser.timezone=UTC"}},
		}
	}
	// The writer is the container's command, so it is PID 1 like the JVM of
	// the Java image, and its records carry the same %pid.
	writer := workloadDir + "/" + pythonWriterScript
	return workloadRuntime{
		kind:            stockWorkload,
		image:           dockerhubMirror + "/" + stockWorkloadImage,
		writerCommand:   []string{"python3", writer},
		ledgerCommand:   ledgerCommand,
		appenderCommand: appenderCommand,
		// The Python writer journals each period before it touches the file,
		// so its ledger comes from that journal rather than from the files,
		// which some rotation modes compress or delete.
		ledgerEnv: []envVar{
			{"LOGWRITER_CRC64_COMMAND", "python3 " + writer + " crc64"},
			{"LOGWRITER_LEDGER_SOURCE", "journal"},
		},
		scripts: map[string]string{
			pythonWriterScript: pythonWriterSource,
			ledgerScript:       ledgerSource,
			appenderScript:     appenderSource,
		},
	}
}

// scriptsChecksum digests the ConfigMap scripts.
func (r workloadRuntime) scriptsChecksum() string {
	names := make([]string, 0, len(r.scripts))
	for name := range r.scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	digest := sha256.New()
	for _, name := range names {
		fmt.Fprintf(digest, "%s\x00%d\x00%s", name, len(r.scripts[name]), r.scripts[name])
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// Fakeintake keeps 15 minutes of payloads by default, which is shorter than a
// full matrix: the last cell checks logs written more than 15 minutes earlier.
// It also forwards everything to a Datadog org by default; a run that leaks
// the SMB account key into a log must not copy it anywhere else.
const fakeintakeRetention = "2h"

// Mount options are selected per cell. Every measurement so far used
// actimeo=1, while Microsoft recommends actimeo=30 to actimeo=60 for Azure
// Files on Linux and documents that lower values cost performance, so real
// deployments are far more likely to run 30 or more. Both are kept so their
// results stay comparable; only the attribute cache lifetime differs.
const (
	mountOptionsActimeo1  = "cache=strict,nosharesock,serverino,actimeo=1,closetimeo=0,persistenthandles"
	mountOptionsActimeo30 = "cache=strict,nosharesock,serverino,actimeo=30,closetimeo=0,persistenthandles"
)

// Agent settings this suite pins, and the drain windows they produce.
//
// Each reader keeps reading a rotated file for a bounded window after it sees
// the rotation, and drops whatever is appended once that window has closed:
//
//   - file source, default profile: the rotated tailer keeps reading next to
//     its replacement for logs_config.close_timeout, then closes its
//     descriptor;
//   - file source, unreliable-mount profile: the rotated tailer hands the path
//     over and stops once its file has gone fileHandoffQuietSeconds without new
//     reads (handoffQuietPeriod in pkg/logs/tailers/file/tailer.go), or at
//     logs_config.unreliable_mount.rotation_drain_timeout, 60s by default;
//   - SMB source: the draining reader polls the rotated file every poll
//     interval and stops once the file has had no new data for
//     logs_config.close_timeout, or smbDrainMaxCloseTimeouts close timeouts
//     after it started (pollDrain in pkg/logs/launchers/smb/scanner.go). It
//     starts in the scan that first lists the renamed file, up to one poll
//     interval and a scan after the rename, and every read of new data
//     restarts the count. An append within close_timeout of its last data is
//     read; one after it is lost without a trace, since no listing watches the
//     file once the drain ended. Before that, the drain ended after two idle
//     polls in a row, one to two poll intervals after the rename.
//
// close_timeout is lowered from its 60s default so that a single marker age
// can land past every window and still come before the next rotation.
const (
	fileScanPeriodSeconds   = 1
	closeTimeoutSeconds     = 5
	fileHandoffQuietSeconds = 30
	smbPollIntervalSeconds  = 1
	// smbDrainMaxCloseTimeouts is how many close timeouts bound an SMB drain
	// (drainMaxCloseTimeouts in pkg/logs/launchers/smb).
	smbDrainMaxCloseTimeouts = 10
	// smbDrainStartLagMs is how long after the rename the SMB drain can
	// start: the scan that first lists the renamed file, up to one poll
	// interval after it, and a second for the scan to reach the file.
	smbDrainStartLagMs = (smbPollIntervalSeconds + 1) * 1000
	// smbDrainEndLagMs is how long after close_timeout a drain can end: it
	// ends at the first poll at or after its start D plus close_timeout, and
	// the polls are one poll interval apart, so up to that long past it.
	smbDrainEndLagMs = smbPollIntervalSeconds * 1000
	// lateAppendLagMs is how long after its age an appender's marker can
	// land: one of its 200ms polls (postRotationMarkerPollMs), an exec and a
	// CIFS append, with room.
	lateAppendLagMs = 1000
)

// Post-rename marker calibration.
//
// The appender sidecar of each cell writes one marker inside its reader's drain
// window and one past every window described above:
//
//   - the surviving marker must always be collected, because it lands while
//     the reader is still reading the rotated file. A file source reads the
//     rotated file for at least close_timeout, so its marker comes at 1.5s.
//     The SMB source reads it until it has had no new data for
//     close_timeout, from the scan that sees the rotation, so its marker
//     comes at 0.5s. It would be collected at any age under close_timeout
//     less the appender's lag (smbLateAppendOutcome); the smb-late-<age>
//     cells probe that. The age predates a drain that ended after two idle
//     polls, one to two poll intervals after the rename, which a 1.5s marker
//     would have lost on about half of the rotations;
//   - the lost marker is the calibration point. It is expected to disappear,
//     which is what proves this suite can observe the loss at all.
//
// Every read of new data restarts the SMB source's close_timeout, so a cell's
// surviving marker also keeps its drain open: one cell cannot probe a later
// age with a third marker.
//
// The windows open when the Agent sees the rotation, not at the rename. A file
// source sees the rename after up to one file scan plus the mount's attribute
// cache lifetime, so with actimeo=30 the window of the file-line-actimeo30 cell
// can open late enough to cover the lost marker in the unreliable-mount
// profile.
//
// If the lost marker starts arriving, treat the run as suspicious rather than
// as a pass and investigate the harness before the product: it usually means
// the marker no longer lands past the drain window (the constants above
// drifted from the Agent, the rotation was detected late so both markers
// slipped, or the surviving marker kept the drain alive longer than expected).
//
// The markers need a renamed file that stays where it was renamed to, so they
// depend on the rotation mode (markerDelaysFor): gzip deletes the rotated file
// after gzipDelayMs and keeps only the surviving marker, or none when a run
// forces losses and compresses at once, and copytruncate and delete-recreate
// leave no renamed file to append to, so their cells have no marker at all.
const (
	fileSurvivingMarkerDelayMs       = 1500
	smbSurvivingMarkerDelayMs        = smbPollIntervalSeconds * 1000 / 2
	postRotationMarkerLostDelayMs    = 45000
	postRotationMarkerSurvivingCount = 1
	postRotationMarkerLostCount      = 0
	postRotationMarkerPollMs         = 200
	postRotationMarkerJournalName    = "markers.jsonl"
)

// markerExpectation is what a cell expects of one of its markers.
type markerExpectation string

const (
	// markerCollected markers must arrive exactly once.
	markerCollected markerExpectation = "collected"
	// markerLost markers must not arrive.
	markerLost markerExpectation = "lost"
	// markerEither markers land within the jitter of the drain's end, so they
	// may arrive on one rotation and not on the next, but never twice.
	markerEither markerExpectation = "either"
)

// markerDelays are the ages, after the rename, at which a cell's appender
// appends its markers to the rotated file, and what is expected of the early
// one; the late one is always expected to be lost. Zero means no such marker.
type markerDelays struct {
	earlyMs     int
	earlyExpect markerExpectation
	lateMs      int
}

// markerDelaysFor returns the marker ages calibrated for a reader's drain
// window, for the markers the writer's rotation leaves a file for.
func markerDelaysFor(reader readerKind, w writerOptions) markerDelays {
	earlyMs := fileSurvivingMarkerDelayMs
	if reader == smbReader {
		earlyMs = smbSurvivingMarkerDelayMs
	}
	switch w.mode {
	case copyTruncateRotation, deleteRecreateRotation:
		return markerDelays{}
	case gzipRotation:
		if w.gzipDelayMs() <= earlyMs {
			// Compressed away before even the early marker's age.
			return markerDelays{}
		}
		// The rotated file is gone long before the lost marker's age.
		return markerDelays{earlyMs: earlyMs, earlyExpect: markerCollected}
	}
	return markerDelays{earlyMs: earlyMs, earlyExpect: markerCollected, lateMs: postRotationMarkerLostDelayMs}
}

// count is the number of markers appended to each rotated file.
func (m markerDelays) count() int {
	count := 0
	for _, delay := range []int{m.earlyMs, m.lateMs} {
		if delay > 0 {
			count++
		}
	}
	return count
}

// expectationFor returns what is expected of the marker of that age, if the
// cell has one.
func (m markerDelays) expectationFor(ageMs int) (markerExpectation, bool) {
	switch {
	case ageMs <= 0:
		return "", false
	case ageMs == m.earlyMs:
		return m.earlyExpect, true
	case ageMs == m.lateMs:
		return markerLost, true
	}
	return "", false
}

// needsSettle reports whether a marker may be expected not to arrive, which
// can only be confirmed once every drain has ended.
func (m markerDelays) needsSettle() bool {
	return m.lateMs > 0 || (m.earlyMs > 0 && m.earlyExpect != markerCollected)
}

// appenderValue is the LOGWRITER_APPEND_DELAYS_MS value of the appender.
func (m markerDelays) appenderValue() string {
	var delays []string
	for _, delay := range []int{m.earlyMs, m.lateMs} {
		if delay > 0 {
			delays = append(delays, strconv.Itoa(delay))
		}
	}
	if len(delays) == 0 {
		return "none"
	}
	return strings.Join(delays, ",")
}

// lateMarkerProbe is the early marker of an smb-late-<age> cell: one age
// probed per cell, since a collected marker restarts the SMB source's
// close_timeout and keeps the drain open for a later one.
type lateMarkerProbe struct {
	ageMs  int
	expect markerExpectation
}

// smbLateAppendOutcome predicts what the SMB source does with a line appended
// ageMs after the rename, from the drain's code (pollDrain in
// pkg/logs/launchers/smb/scanner.go).
//
// The scan that first lists the renamed file, at D, starts the drain, and its
// idle count starts at D. D is up to smbDrainStartLagMs after the rename R.
// The drain ends at the first poll at or after D plus close_timeout, once
// close_timeout has passed with no new data: that poll is up to
// smbDrainEndLagMs after D plus close_timeout, so the end is between R plus
// close_timeout and R plus close_timeout plus smbDrainStartLagMs plus
// smbDrainEndLagMs (8s with close_timeout 5s, a poll interval of 1s and a
// scan margin of 1s).
//
// Until it goes quiet, the drain reads the file only when the listing shows it
// changed, or every ForceReadEvery polls, since listing sizes can be stale.
// Once it is quiet it reads the file one last time whatever the listing shows,
// and ends unless that read finds new data, which restarts the idle count.
// What guarantees that an append that landed before the end is collected is
// that last read, not the polls before it. The appender appends at R plus the
// age plus a lag of at most lateAppendLagMs. So an append that lands before
// the earliest end of the drain is collected, one that comes after its
// latest end is lost, and one in between may go either way. A lost append is
// lost without a trace: no listing shows it, so nothing is reported missed.
//
// The probe markers' outcomes stay predictions until runs with
// AZURE_FILES_E2E_CALIBRATE=1 confirm them (see README.md).
//
// TODO(calibrate): run the three cells with AZURE_FILES_E2E_CALIBRATE=1 and
// compare. The predictions for 1s, 2s and 3s are all collected: before
// the drain kept reading for close_timeout, a calibration run collected the 1s
// marker 2 times out of 3 and lost the 2s and 3s ones.
func smbLateAppendOutcome(ageMs int) markerExpectation {
	switch {
	case ageMs+lateAppendLagMs < closeTimeoutSeconds*1000:
		return markerCollected
	case ageMs > closeTimeoutSeconds*1000+smbDrainStartLagMs+smbDrainEndLagMs:
		return markerLost
	}
	return markerEither
}

// lateMarkerProbes are the smb-late-<age> cells' early marker ages and the
// outcome each one asserts unless AZURE_FILES_E2E_CALIBRATE=1 is set. All
// three ages are under close_timeout, so the source must collect each one.
//
// The slack of each is what the append can lag beyond its age before it lands
// after the earliest end of a drain (R+close_timeout, when the drain started
// right at the rename): 4s, 3s and 2s, against lateAppendLagMs of 1s. An
// append that stalls on the share for more than 2s lands after that end and
// makes the 3000ms cell lose a marker a correct source cannot collect, so a
// failure of that cell alone, with the appender's journal showing a slow
// append, is the share, not the product. The journal's appended_at has a
// second's resolution, too coarse for the test to tell.
//
// They catch a close_timeout below 4s only by chance: such a drain can end
// before the 3s probe lands (as late as R+4s) when it started right at the
// rename, and does not when it started up to 2s later. No probe can pin the
// value from either side without flaking, because the end of a drain moves by
// more than the band that separates two values. With close_timeout C, a drain
// ends between R+C and R+C plus smbDrainStartLagMs plus smbDrainEndLagMs, so
// a drain of 5s and one of 4s can both end anywhere from R+5s to R+7s. A probe
// in the 4000 to 5000ms band lands between R+4s and R+6s, when both can have
// ended and neither has to have: it is markerEither, which asserts nothing. The
// tests that pin close_timeout are those of the drain with a mock clock
// (pkg/logs/launchers/smb).
var lateMarkerProbes = []lateMarkerProbe{
	{ageMs: 1000, expect: smbLateAppendOutcome(1000)},
	{ageMs: 2000, expect: smbLateAppendOutcome(2000)},
	{ageMs: 3000, expect: smbLateAppendOutcome(3000)},
}

// lateMarkerCellName names the smb-late-<age> cell of a probe.
func lateMarkerCellName(probe lateMarkerProbe) string {
	return "smb-late-" + strconv.Itoa(probe.ageMs)
}

// Writer options. A run applies one writer configuration to every cell, like
// the profile on the Agent side, and the defaults are the Java writer's:
// rename rotation every minute, each period filled to its target size at once,
// and one app.log at the root of the share. Only the stock Python writer
// supports anything else.
type rotationMode string

const (
	// renameRotation is Log4j2's RollingFile: close, rename, reopen.
	renameRotation rotationMode = "rename"
	// copyTruncateRotation is logrotate's copytruncate: copy app.log, then
	// truncate it in place while the writer keeps appending.
	copyTruncateRotation rotationMode = "copytruncate"
	// deleteRecreateRotation closes app.log, deletes it after a pause, and
	// creates a new one at the same path.
	deleteRecreateRotation rotationMode = "delete-recreate"
	// gzipRotation renames app.log, then compresses the rotated file to .gz
	// and deletes it.
	gzipRotation rotationMode = "gzip"
)

var knownRotationModes = []rotationMode{renameRotation, copyTruncateRotation, deleteRecreateRotation, gzipRotation}

const (
	defaultWriterPeriodMs = 60000
	minWriterPeriodMs     = 5000
	maxWriterPeriodMs     = 600000
	// Each stream adds a directory that the appender lists five times a
	// second and the ledger once a second, and Azure bills every listing.
	maxWriterStreams         = 32
	maxWriterRateBytesPerSec = 20 * 1000 * 1000

	// The Java writer's head pause and first-period runway; a shorter period
	// scales them down to a sixth of itself.
	defaultHeadPauseMs         = 5000
	defaultInitialFillRunwayMs = 10000

	// A paced writer batches records into writes of up to this size, like a
	// Log4j2 appender with a buffer and no immediateFlush, and gives each
	// record a 1 KiB payload, so a high rate takes fewer writes and records.
	pacedWriterBufferBytes  = 64 * 1024
	pacedWriterPayloadBytes = 1024
	// A paced writer keeps at least this many rotated files, and at least
	// the last two minutes of them, so a kept stack stays within the share
	// quota while every drain and marker is long over.
	minRetainedRotations = 6
	retainedRotationsMs  = 120000
	// pacedWriterMaxPeriods is how many periods a paced writer writes before
	// it idles (LOGWRITER_MAX_PERIODS): the completedFiles a cell waits for,
	// the period that rotates the last of them, and two to spare for a late
	// start. A kept stack then stops writing, shipping and billing writes
	// once the test is over, and keeps the files for investigation.
	pacedWriterMaxPeriods = 7

	// writerTickMs is the writers' fixed delay between two ticks
	// (LOGWRITER_INTERVAL_MS, logwriter.interval-ms of the Java writer).
	writerTickMs = 250

	// deleteRecreatePauseMs is how long the closed app.log stays before the
	// writer deletes it and creates the next one.
	deleteRecreatePauseMs = 1000
	// gzipDelayMs is how long a rotated file stays before it is compressed
	// and deleted. The surviving marker must land before.
	gzipDelayMs = 5000
	// copyTruncateHoldMs is how long app.log keeps the copied records before
	// it is truncated: a record written before the copy started stayed in
	// app.log for at least this long, longer than a file scan or an SMB poll.
	copyTruncateHoldMs = 3000
	// copyTruncateSMBAtRiskMs is how long before a truncation a record must
	// have been written to be at risk for an SMB source
	// (LOGWRITER_COPYTRUNCATE_AT_RISK_MS): one poll interval, and half of one
	// for the scan to reach the file. Anything written earlier in the hold
	// was in app.log for a whole poll, so the source must collect it. A file
	// source reads through the mount's attribute cache, so every record of
	// the hold stays at risk for it.
	copyTruncateSMBAtRiskMs = smbPollIntervalSeconds*1000 + smbPollIntervalSeconds*1000/2

	// shareQuotaGiB is the size of every cell's share. A paced writer's
	// retained files, its active files and one file being rotated must fit
	// in 80% of it.
	shareQuotaGiB = 5
)

// writerOptions configure what every writer of a run writes.
type writerOptions struct {
	mode     rotationMode
	periodMs int
	// rateBytesPerSec is each writer pod's total rate, shared by its streams.
	// Zero keeps the Java writer's schedule: each period is filled to its
	// target size right after the head pause.
	rateBytesPerSec int
	// streams is the number of app.log files, under svc-1 to svc-<N>. Zero
	// writes one app.log at the root of the share.
	streams int
	// forceLoss removes the pause of a gzip or delete-recreate rotation, so
	// the rotated file goes away before the source has read its end and the
	// loss accounting has losses to check (AZURE_FILES_E2E_FORCE_LOSS).
	forceLoss bool
}

// gzipDelayMs is how long a gzip rotation keeps the rotated file before it
// compresses it away: none when the run forces losses.
func (w writerOptions) gzipDelayMs() int {
	if w.forceLoss {
		return 0
	}
	return gzipDelayMs
}

// deletePauseMs is how long a delete-recreate rotation keeps the closed
// app.log before it deletes it: none when the run forces losses.
func (w writerOptions) deletePauseMs() int {
	if w.forceLoss {
		return 0
	}
	return deleteRecreatePauseMs
}

func defaultWriterOptions() writerOptions {
	return writerOptions{mode: renameRotation, periodMs: defaultWriterPeriodMs}
}

// writerOptionsSet says which writer options a run sets itself. The ones it
// leaves unset take a scenario's or a cell's defaults when they have some.
type writerOptionsSet struct {
	mode, period, rate, streams bool
}

func parseWriterOptions(opts runOptions) (writerOptions, writerOptionsSet, error) {
	w := defaultWriterOptions()
	set := writerOptionsSet{
		mode:    strings.TrimSpace(opts.rotationMode) != "",
		period:  strings.TrimSpace(opts.periodMs) != "",
		rate:    strings.TrimSpace(opts.rateBytesPerSec) != "",
		streams: strings.TrimSpace(opts.streams) != "",
	}
	if mode := strings.TrimSpace(opts.rotationMode); mode != "" {
		w.mode = rotationMode(mode)
		if !slices.Contains(knownRotationModes, w.mode) {
			names := make([]string, 0, len(knownRotationModes))
			for _, known := range knownRotationModes {
				names = append(names, string(known))
			}
			return writerOptions{}, set, fmt.Errorf("unknown rotation mode %q; known modes are %s", mode, strings.Join(names, ","))
		}
	}
	var err error
	if w.periodMs, err = parseBoundedInt("writer period", opts.periodMs, defaultWriterPeriodMs, minWriterPeriodMs, maxWriterPeriodMs); err != nil {
		return writerOptions{}, set, err
	}
	if w.periodMs%1000 != 0 {
		return writerOptions{}, set, fmt.Errorf("writer period %dms must be whole seconds", w.periodMs)
	}
	if w.rateBytesPerSec, err = parseBoundedInt("writer rate", opts.rateBytesPerSec, 0, 0, maxWriterRateBytesPerSec); err != nil {
		return writerOptions{}, set, err
	}
	if w.streams, err = parseBoundedInt("writer streams", opts.streams, 0, 0, maxWriterStreams); err != nil {
		return writerOptions{}, set, err
	}
	return w, set, nil
}

// validate refuses a period out of range, which a scenario's or a cell's
// default could give too, and a paced writer whose files would not fit in its
// share.
func (w writerOptions) validate() error {
	if w.periodMs < minWriterPeriodMs || w.periodMs > maxWriterPeriodMs {
		return fmt.Errorf("writer period %dms must be between %d and %d", w.periodMs, minWriterPeriodMs, maxWriterPeriodMs)
	}
	if !w.paced() {
		return nil
	}
	periodBytes := int64(w.rateBytesPerSec) * int64(w.periodMs) / 1000
	if needed := int64(w.retainedRotations()+2) * periodBytes; needed > (shareQuotaGiB<<30)*8/10 {
		return fmt.Errorf("writer rate %d with a %dms period keeps %d MB on the share, more than its %d GiB quota allows; shorten the period or lower the rate",
			w.rateBytesPerSec, w.periodMs, needed/1000/1000, shareQuotaGiB)
	}
	return nil
}

// writerDefaults are a cell's or a scenario's own writer options, which apply
// when the run leaves the option unset. Zero leaves the run's value.
type writerDefaults struct {
	periodMs        int
	rateBytesPerSec int
	streams         int
}

// withDefaults applies the defaults to the options the run left unset.
func (w writerOptions) withDefaults(set writerOptionsSet, d writerDefaults) writerOptions {
	if !set.period && d.periodMs != 0 {
		w.periodMs = d.periodMs
	}
	if !set.rate && d.rateBytesPerSec != 0 {
		w.rateBytesPerSec = d.rateBytesPerSec
	}
	if !set.streams && d.streams != 0 {
		w.streams = d.streams
	}
	return w
}

func parseBoundedInt(what, value string, fallback, minimum, maximum int) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s %q is not a number", what, value)
	}
	if parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s %d must be between %d and %d", what, parsed, minimum, maximum)
	}
	return parsed, nil
}

func (w writerOptions) isDefault() bool {
	return w == defaultWriterOptions()
}

func (w writerOptions) paced() bool {
	return w.rateBytesPerSec > 0
}

func (w writerOptions) headPauseMs() int {
	if w.periodMs == defaultWriterPeriodMs {
		return defaultHeadPauseMs
	}
	return min(defaultHeadPauseMs, w.periodMs/6)
}

func (w writerOptions) initialFillRunwayMs() int {
	if w.periodMs == defaultWriterPeriodMs {
		return defaultInitialFillRunwayMs
	}
	return min(defaultInitialFillRunwayMs, w.periodMs/6)
}

// retainedRotations is LOGWRITER_MAX_ROTATED_FILES: zero keeps every file.
func (w writerOptions) retainedRotations() int {
	if !w.paced() {
		return 0
	}
	return max(minRetainedRotations, (retainedRotationsMs+w.periodMs-1)/w.periodMs)
}

// streamDirs are the directories of the writer's streams, relative to the
// share root; "" is the root itself.
func (w writerOptions) streamDirs() []string {
	if w.streams == 0 {
		return []string{""}
	}
	dirs := make([]string, 0, w.streams)
	for i := 1; i <= w.streams; i++ {
		dirs = append(dirs, streamDirPrefix+strconv.Itoa(i))
	}
	return dirs
}

// sharePaths are the paths of a file of every stream on the writer's mount.
func (w writerOptions) sharePaths(name string) []string {
	var paths []string
	for _, dir := range w.streamDirs() {
		paths = append(paths, path.Join(logMountPath, dir, name))
	}
	return paths
}

// activeLogPattern is the path of the active files relative to the share
// root, as a log source matches it.
func (w writerOptions) activeLogPattern() string {
	if w.streams == 0 {
		return activeLogName
	}
	return "*/" + activeLogName
}

// streamEnv tells the ledger and the appender where the streams are.
func (w writerOptions) streamEnv() []envVar {
	if w.streams == 0 {
		return nil
	}
	return []envVar{{"LOGWRITER_STREAMS", strconv.Itoa(w.streams)}}
}

// writerEnv is what the Python writer needs beyond the Java writer's
// configuration; it is empty for the defaults.
func (w writerOptions) writerEnv() []envVar {
	var vars []envVar
	switch w.mode {
	case copyTruncateRotation:
		vars = append(vars, envVar{"LOGWRITER_ROTATION_MODE", string(w.mode)},
			envVar{"LOGWRITER_COPYTRUNCATE_HOLD_MS", strconv.Itoa(copyTruncateHoldMs)})
	case deleteRecreateRotation:
		vars = append(vars, envVar{"LOGWRITER_ROTATION_MODE", string(w.mode)},
			envVar{"LOGWRITER_DELETE_PAUSE_MS", strconv.Itoa(w.deletePauseMs())})
	case gzipRotation:
		vars = append(vars, envVar{"LOGWRITER_ROTATION_MODE", string(w.mode)},
			envVar{"LOGWRITER_GZIP_DELAY_MS", strconv.Itoa(w.gzipDelayMs())})
	}
	if w.periodMs != defaultWriterPeriodMs {
		vars = append(vars, envVar{"LOGWRITER_PERIOD_MS", strconv.Itoa(w.periodMs)},
			envVar{"LOGWRITER_INITIAL_FILL_RUNWAY_MS", strconv.Itoa(w.initialFillRunwayMs())})
	}
	if w.paced() {
		vars = append(vars,
			envVar{"LOGWRITER_RATE_BYTES_PER_SEC", strconv.Itoa(w.rateBytesPerSec)},
			envVar{"LOGWRITER_BUFFER_BYTES", strconv.Itoa(pacedWriterBufferBytes)},
			envVar{"LOGWRITER_PAYLOAD_BYTES", strconv.Itoa(pacedWriterPayloadBytes)},
			// At these rates the records would flood the container log.
			envVar{"LOGWRITER_CONSOLE_RECORDS", "false"},
			envVar{"LOGWRITER_MAX_ROTATED_FILES", strconv.Itoa(w.retainedRotations())},
			envVar{"LOGWRITER_MAX_PERIODS", strconv.Itoa(pacedWriterMaxPeriods)},
		)
	}
	return append(vars, w.streamEnv()...)
}

// metadata describes the options in the run metadata.
func (w writerOptions) metadata() map[string]any {
	metadata := map[string]any{
		"rotation_mode":      w.mode,
		"period_ms":          w.periodMs,
		"rate_bytes_per_sec": w.rateBytesPerSec,
		"streams":            w.streams,
		"head_pause_ms":      w.headPauseMs(),
		"active_log_pattern": w.activeLogPattern(),
	}
	switch w.mode {
	case copyTruncateRotation:
		metadata["copytruncate_hold_ms"] = copyTruncateHoldMs
	case deleteRecreateRotation:
		metadata["delete_pause_ms"] = w.deletePauseMs()
	case gzipRotation:
		metadata["gzip_delay_ms"] = w.gzipDelayMs()
	}
	if w.forceLoss {
		metadata["force_loss"] = true
	}
	if w.paced() {
		metadata["buffer_bytes"] = pacedWriterBufferBytes
		metadata["payload_bytes"] = pacedWriterPayloadBytes
		metadata["max_rotated_files"] = w.retainedRotations()
		metadata["max_periods"] = pacedWriterMaxPeriods
	}
	return metadata
}

// readerKind is the Agent log source that collects a cell's share.
type readerKind string

const (
	// fileReader is a file log source over the Agent's own CIFS mount.
	fileReader readerKind = "file"
	// smbReader is the native SMB log source, which reads the share directly.
	smbReader readerKind = "smb"
)

// fingerprintConfig is the per-source fingerprint_config of a file reader.
type fingerprintConfig struct {
	strategy string
	count    int
	maxBytes int
}

// fileServerKind is what serves a cell's share.
type fileServerKind string

const (
	// azureFilesServer is an Azure Files share of the cell's own storage
	// account, written by a writer pod through its CIFS mount.
	azureFilesServer fileServerKind = "azure-files"
	// windowsFileServer is a share of the run's Windows Server VM, written
	// by a writer that runs on the VM itself (see windows.go).
	windowsFileServer fileServerKind = "windows"
)

// cell is one independent measurement. Each cell has its own storage account,
// share and writer, so no two readers ever see the same files.
type cell struct {
	name        string
	reader      readerKind
	server      fileServerKind
	service     string
	fingerprint fingerprintConfig
	// mountOptions applies to the writer's mount, and to the Agent's mount
	// when the reader is a file source.
	mountOptions string
	markers      markerDelays
	shareName    string
	accountName  string
	volumeName   string
	writerName   string
	// writer is what this cell's writer writes: the run's writer options,
	// with the cell's fixed mode and its defaults for the unset options.
	writer writerOptions

	// inDefaultMatrix cells run when AZURE_FILES_E2E_CELLS is unset; the
	// others only run when named.
	inDefaultMatrix bool
	// fixedMode is the rotation mode the cell always uses, whatever the run
	// sets; empty takes the run's.
	fixedMode rotationMode
	// writerDefaults are the cell's own writer options for those the run
	// leaves unset.
	writerDefaults writerDefaults
	// probe is the early marker of an smb-late-<age> cell.
	probe *lateMarkerProbe
	// agentKeyIndex is the storage account key the Agent's copy of the
	// cell's Secret holds: 0 for key1, which the writer mounts with, and 1
	// for key2 in the key-rotation scenario.
	agentKeyIndex int
	// serverHost is the private IP address of the Windows file server, which
	// the test learns from the storage pass, for a cell it serves.
	serverHost string
}

// onWindows reports whether the Windows file server serves the cell's share.
func (c cell) onWindows() bool {
	return c.server == windowsFileServer
}

// lossAccounted reports whether the cell's records may only go missing when
// the Agent reported the bytes as missed: an SMB source whose rotation mode
// deletes rotated files (gzip, delete-recreate) can only miss what it did not
// read before the file went away, and must say so.
func (c cell) lossAccounted() bool {
	return c.reader == smbReader && (c.writer.mode == gzipRotation || c.writer.mode == deleteRecreateRotation)
}

// secretName is the Kubernetes Secret holding the cell's storage account
// credentials. The CSI driver reads it in the e2e namespace for the CIFS
// mounts. For an SMB cell, a copy of the key under the same name in the Agent
// namespace is mounted into the Agent pod.
func (c cell) secretName() string {
	return storageSecretPrefix + "-" + c.name
}

// host is the SMB server of the cell's share.
func (c cell) host() string {
	if c.onWindows() {
		return c.serverHost
	}
	return c.accountName + ".file.core.windows.net"
}

// smbUsername is the user the SMB source authenticates as: the storage
// account for Azure Files, a local user of the Windows file server.
func (c cell) smbUsername() string {
	if c.onWindows() {
		return windowsReaderUser
	}
	return c.accountName
}

// agentKeyVolumeName is the Agent pod volume that holds an SMB cell's key.
func (c cell) agentKeyVolumeName() string {
	return c.volumeName + "-key"
}

// agentKeyDir is where the Agent pod mounts an SMB cell's key.
func agentKeyDir(c cell) string {
	return agentSecretsMountRoot + "/" + c.name
}

// agentProfile is the node-wide Agent configuration of a run.
// logs_config.unreliable_mount applies to every file source on the Agent and
// the suite runs one Agent, so one run covers one profile and comparing the
// profiles takes one run each.
type agentProfile struct {
	name            string
	unreliableMount bool
}

var (
	defaultProfile         = agentProfile{name: "default"}
	unreliableMountProfile = agentProfile{name: "unreliable-mount", unreliableMount: true}
	knownProfiles          = []agentProfile{defaultProfile, unreliableMountProfile}
)

func parseProfile(name string) (agentProfile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultProfile, nil
	}
	names := make([]string, 0, len(knownProfiles))
	for _, profile := range knownProfiles {
		if profile.name == name {
			return profile, nil
		}
		names = append(names, profile.name)
	}
	return agentProfile{}, fmt.Errorf("unknown profile %q; known profiles are %s", name, strings.Join(names, ","))
}

// SMB client timings the scenarios are sized against, from
// pkg/logs/internal/smb/client: each ListDir or ReadAt call is bounded by
// defaultOpTimeout, and a dial by defaultDialTimeout. After a lost session,
// failed dials back off up to maxBackoff, and a dial that fails
// authentication waits maxBackoff before the next one.
const (
	smbOpTimeoutSeconds   = 30
	smbDialTimeoutSeconds = 10
	smbMaxBackoffSeconds  = 30
)

// Log pipeline timings the agent-restart duplicate bound is derived from: the
// auditor writes the registry every defaultFlushPeriod
// (comp/logs/auditor/impl/auditor.go) and once more when it stops, and the
// sender sends a batch when it is full or logs_config.batch_wait after its
// first message.
const (
	auditorFlushPeriodSeconds = 1
	logsBatchWaitSeconds      = 5
)

// scenarioKind names a scenario, which runs as its own test method on the smb
// cell (see scenarios_test.go): a disruption of the Agent, or several Agents.
type scenarioKind string

const (
	noScenario           scenarioKind = ""
	agentRestartScenario scenarioKind = "agent-restart"
	keyRotationScenario  scenarioKind = "key-rotation"
	networkDropScenario  scenarioKind = "network-drop"
	// multiNodeScenario runs the Agent DaemonSet on several AKS nodes, all
	// with the same smb source.
	multiNodeScenario scenarioKind = "multi-node"
)

var (
	// disruptionScenarios disrupt the Agent while the smb cell's writer runs.
	disruptionScenarios = []scenarioKind{agentRestartScenario, keyRotationScenario, networkDropScenario}
	knownScenarios      = append(slices.Clone(disruptionScenarios), multiNodeScenario)
)

// disrupts reports whether the scenario disrupts the Agent.
func (k scenarioKind) disrupts() bool {
	return slices.Contains(disruptionScenarios, k)
}

// scenarioCellName is the cell every scenario runs on.
const scenarioCellName = "smb"

// AKS node counts. The multi-node scenario runs one Agent per node, so every
// record of the smb cell is collected once per node: the SMB source has no
// single-reader election, each Agent with the source reads the whole share.
// Any other cell would be collected as many times, so only that scenario may
// run more than one node.
const (
	defaultNodes          = 1
	defaultMultiNodeNodes = 2
	maxNodes              = 5
)

// Scenario timings. Each scenario disrupts the second period of the smb
// cell's writer, the first full one, and the disruption has to be over before
// that period ends: a file that rotates in and out while the source cannot
// read the share is never read (see the product notes in README.md). So each
// scenario has a minimum period, and a default period that leaves a margin.
// The writer is paced, so the active file keeps growing through the
// disruption and the source has to catch up on it.
const (
	// agent-restart: the Agent pod is deleted this far into the period,
	// once the source has read part of the active file. The replacement must
	// be running the source again before the period ends.
	agentRestartPeriodMs        = 120000
	agentRestartMinPeriodMs     = 120000
	agentRestartRateBytesPerSec = 20000
	agentRestartDeleteAfterMs   = 30000

	// key-rotation: key2 is renewed this far into the period. The Agent then
	// has to fail authentication (on its own if Azure drops its session,
	// else after keyRotationForcedDropSeconds without SMB traffic), the
	// Secret update has to reach the Agent pod (up to about a minute on
	// AKS), and the secret refresh has to pick it up.
	keyRotationPeriodMs          = 300000
	keyRotationMinPeriodMs       = 240000
	keyRotationRateBytesPerSec   = 10000
	keyRotationStartAfterMs      = 15000
	secretRefreshIntervalSeconds = 15
	keyRotationNaturalAuthWait   = 45 * time.Second
	keyRotationForcedDropSeconds = smbOpTimeoutSeconds + 5

	// network-drop: the Agent pod's SMB traffic is dropped for
	// AZURE_FILES_E2E_NETWORK_DROP_SECONDS around the end of the period, so
	// the file rotates while the source cannot reach the share. The drop must
	// outlast an operation timeout, so the client loses its session.
	defaultNetworkDropSeconds  = 90
	minNetworkDropSeconds      = smbOpTimeoutSeconds + 1
	networkDropPeriodMs        = 180000
	networkDropRateBytesPerSec = 10000
	// The period must hold the drop, the client's longest backoff after it,
	// and a margin, so that only one rotation happens during the outage.
	networkDropMarginSeconds = smbMaxBackoffSeconds + 60
	// maxNetworkDropSeconds is the longest drop whose period, with that
	// margin, is still a writer period the suite accepts: 510s.
	maxNetworkDropSeconds = maxWriterPeriodMs/1000 - networkDropMarginSeconds
)

// scenarioOptions are the scenario of a run.
type scenarioOptions struct {
	kind scenarioKind
	// dropSeconds is how long network-drop blocks the Agent's SMB traffic.
	dropSeconds int
}

func parseScenario(name, dropSeconds string) (scenarioOptions, error) {
	s := scenarioOptions{kind: scenarioKind(strings.TrimSpace(name))}
	if s.kind != noScenario && !slices.Contains(knownScenarios, s.kind) {
		names := make([]string, 0, len(knownScenarios))
		for _, known := range knownScenarios {
			names = append(names, string(known))
		}
		return scenarioOptions{}, fmt.Errorf("unknown scenario %q; known scenarios are %s", s.kind, strings.Join(names, ","))
	}
	if s.kind != networkDropScenario {
		if strings.TrimSpace(dropSeconds) != "" {
			return scenarioOptions{}, fmt.Errorf("a network drop duration only applies to the %s scenario", networkDropScenario)
		}
		return s, nil
	}
	var err error
	s.dropSeconds, err = parseBoundedInt("network drop", dropSeconds, defaultNetworkDropSeconds, minNetworkDropSeconds, maxNetworkDropSeconds)
	return s, err
}

// minPeriodMs is the shortest writer period that holds the disruption.
func (s scenarioOptions) minPeriodMs() int {
	switch s.kind {
	case agentRestartScenario:
		return agentRestartMinPeriodMs
	case keyRotationScenario:
		return keyRotationMinPeriodMs
	case networkDropScenario:
		return (s.dropSeconds + networkDropMarginSeconds) * 1000
	}
	return 0
}

// writerDefaults are the scenario's writer period and rate.
func (s scenarioOptions) writerDefaults() writerDefaults {
	switch s.kind {
	case agentRestartScenario:
		return writerDefaults{periodMs: agentRestartPeriodMs, rateBytesPerSec: agentRestartRateBytesPerSec}
	case keyRotationScenario:
		return writerDefaults{periodMs: keyRotationPeriodMs, rateBytesPerSec: keyRotationRateBytesPerSec}
	case networkDropScenario:
		return writerDefaults{periodMs: max(networkDropPeriodMs, s.minPeriodMs()), rateBytesPerSec: networkDropRateBytesPerSec}
	}
	return writerDefaults{}
}

// validateWriter refuses writer options the scenario cannot judge. Every
// scenario rotates by rename. A disruption reads one app.log, written at a
// rate, with a period that holds the disruption.
func (s scenarioOptions) validateWriter(w writerOptions) error {
	switch {
	case s.kind == noScenario:
		return nil
	case w.mode != renameRotation:
		return fmt.Errorf("the %s scenario rotates by rename; unset the rotation mode", s.kind)
	case !s.kind.disrupts():
		return nil
	case w.streams != 0:
		return fmt.Errorf("the %s scenario reads one app.log; unset the writer streams", s.kind)
	case !w.paced():
		return fmt.Errorf("the %s scenario needs a paced writer, so the active file grows through the disruption; set a writer rate above 0", s.kind)
	case w.periodMs < s.minPeriodMs():
		return fmt.Errorf("the %s scenario needs a writer period of at least %dms to finish its disruption within one period, not %dms", s.kind, s.minPeriodMs(), w.periodMs)
	}
	return nil
}

// checkCells refuses cells the scenario would disturb or could not judge.
// agent-restart and key-rotation disturb every source of the Agent: a
// restart restarts them all, and a secret refresh schedules the whole
// azure_files configuration again. network-drop only blocks the smb cell's
// storage endpoint inside the Agent pod's network namespace, so the file
// cells, whose CIFS mounts the kernel runs in the node's namespace, are not
// affected and keep their usual assertions. Other SMB cells are refused:
// several storage accounts can share the blocked endpoint's IP address.
// multi-node runs an Agent per node, which would read every other cell once
// per node too.
func (s scenarioOptions) checkCells(cells []cell) error {
	if s.kind == noScenario {
		return nil
	}
	found := false
	for _, c := range cells {
		switch {
		case c.name == scenarioCellName:
			found = true
		case s.kind == networkDropScenario && c.reader == fileReader:
		case s.kind == networkDropScenario:
			return fmt.Errorf("the %s scenario cannot run with cell %s: SMB cells may share the blocked storage endpoint's IP address", s.kind, c.name)
		case s.kind == multiNodeScenario:
			return fmt.Errorf("the %s scenario runs an Agent on every node, each collecting every cell, so it runs with the %s cell alone, not with %s", s.kind, scenarioCellName, c.name)
		default:
			return fmt.Errorf("the %s scenario disturbs every source of the Agent, so it runs with the %s cell alone, not with %s", s.kind, scenarioCellName, c.name)
		}
	}
	if !found {
		return fmt.Errorf("the %s scenario runs on the %s cell; add it to the selected cells", s.kind, scenarioCellName)
	}
	return nil
}

var (
	// stackNamePattern keeps a reused stack name valid as a Pulumi stack name
	// once the framework prefixes it with the user name.
	stackNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)
	// nodeSuffixPattern is the suffix the suite gives the stack of a
	// multi-node run, which a reused stack name must not carry itself.
	nodeSuffixPattern = regexp.MustCompile(`-n[0-9]+$`)
)

// runOptions are the inputs of one run, read from the environment by
// TestAzureFiles.
type runOptions struct {
	runID       string
	writerImage string
	profile     string
	// cells is a comma-separated cell filter; empty selects every cell.
	cells string
	// stackName reuses one stack across runs. Empty gives each run its own.
	stackName string
	// smbEnabled provisions the smb cell, which needs an Agent built with the
	// native SMB log source.
	smbEnabled bool
	// The writer options, as given; empty keeps each default.
	rotationMode    string
	periodMs        string
	rateBytesPerSec string
	streams         string
	// calibrate records the outcome of each smb-late-<age> cell's probe
	// marker instead of asserting it.
	calibrate bool
	// forceLoss makes the gzip and delete-recreate rotations of the
	// loss-accounted cells remove the rotated file at once.
	forceLoss bool
	// scenario names a scenario to run on the smb cell, and
	// networkDropSeconds how long network-drop lasts; empty keeps 90s.
	scenario           string
	networkDropSeconds string
	// nodes is the AKS node count; empty keeps 1, or 2 for multi-node.
	nodes string
	// windowsEnabled provisions the Windows file server of the smb-windows
	// cell.
	windowsEnabled bool
}

type runSpec struct {
	runID       string
	stackName   string
	writerImage string
	profile     agentProfile
	// nodes is the number of AKS nodes, and so of Agent pods.
	nodes int
	// writer is the run's writer options. Each cell's are in cell.writer.
	writer    writerOptions
	calibrate bool
	scenario  scenarioOptions
	// cells are provisioned and asserted on.
	cells []cell
	// gatedCells were selected but are left out of the stack because their
	// opt-in is not set. The test reports them as skipped.
	gatedCells []cell
}

// errJavaWriterOptions explains why the Java writer image refuses options.
var errJavaWriterOptions = errors.New("the Java writer image only rotates by rename every minute at the root of the share; " +
	"other rotation modes, periods, rates and streams need the stock Python writer")

func newRunSpec(opts runOptions) (runSpec, error) {
	profile, err := parseProfile(opts.profile)
	if err != nil {
		return runSpec{}, err
	}
	scenario, err := parseScenario(opts.scenario, opts.networkDropSeconds)
	if err != nil {
		return runSpec{}, err
	}
	nodes, err := parseNodes(opts.nodes, scenario)
	if err != nil {
		return runSpec{}, err
	}
	stackName, err := resolveStackName(opts.stackName, opts.runID, nodes)
	if err != nil {
		return runSpec{}, err
	}
	writer, set, err := parseWriterOptions(opts)
	if err != nil {
		return runSpec{}, err
	}
	// A scenario sets the run's period and rate unless the run does; no cell
	// it runs with has defaults of its own.
	writer = writer.withDefaults(set, scenario.writerDefaults())
	if err := writer.validate(); err != nil {
		return runSpec{}, err
	}
	if err := scenario.validateWriter(writer); err != nil {
		return runSpec{}, err
	}
	if opts.writerImage != "" && !writer.isDefault() {
		return runSpec{}, errJavaWriterOptions
	}

	cellFilter := opts.cells
	if scenario.kind != noScenario && strings.TrimSpace(cellFilter) == "" {
		cellFilter = scenarioCellName
	}
	selected, err := filterCells(allCells(stackName, opts.runID), cellFilter)
	if err != nil {
		return runSpec{}, err
	}
	if err := scenario.checkCells(selected); err != nil {
		return runSpec{}, err
	}
	spec := runSpec{
		runID:       opts.runID,
		stackName:   stackName,
		writerImage: opts.writerImage,
		profile:     profile,
		nodes:       nodes,
		writer:      writer,
		calibrate:   opts.calibrate,
		scenario:    scenario,
	}
	probed, forced := false, false
	for _, c := range selected {
		if c.writer, err = c.writerOptions(writer, set); err != nil {
			return runSpec{}, err
		}
		if opts.forceLoss && c.lossAccounted() {
			// The Java schedule fills each file right after its head
			// pause, so nothing is left to lose when the file goes away.
			if !c.writer.paced() {
				return runSpec{}, fmt.Errorf("cell %s: forced losses need a paced writer, whose file still grows when it rotates; "+
					"set AZURE_FILES_E2E_RATE_BYTES_PER_SEC, for example to 200000 with AZURE_FILES_E2E_PERIOD_MS=10000", c.name)
			}
			c.writer.forceLoss = true
			forced = true
		}
		probed = probed || c.probe != nil
		if c.onWindows() && !c.writer.isDefault() {
			return runSpec{}, fmt.Errorf("cell %s: %w", c.name, errWindowsWriterOptions)
		}
		if opts.writerImage != "" && !c.writer.isDefault() {
			return runSpec{}, fmt.Errorf("cell %s: %w", c.name, errJavaWriterOptions)
		}
		switch {
		case scenario.kind.disrupts() && c.name == scenarioCellName:
			// A disruption delays the drains past every marker age, so the
			// markers would say nothing about the drain window.
			c.markers = markerDelays{}
			if scenario.kind == keyRotationScenario {
				c.agentKeyIndex = 1
			}
		case c.probe != nil:
			c.markers = markerDelays{earlyMs: c.probe.ageMs, earlyExpect: c.probe.expect, lateMs: postRotationMarkerLostDelayMs}
		default:
			c.markers = markerDelaysFor(c.reader, c.writer)
		}
		// An SMB cell needs an Agent built with the SMB source, and the
		// smb-windows cell a Windows VM, which costs more than a share.
		if (c.reader == smbReader && !opts.smbEnabled) || (c.onWindows() && !opts.windowsEnabled) {
			spec.gatedCells = append(spec.gatedCells, c)
			continue
		}
		spec.cells = append(spec.cells, c)
	}
	// Calibration only relaxes the probe marker of the smb-late-<age> cells.
	// Left set for any other run, it would make that run look calibrated.
	if opts.calibrate && !probed {
		return runSpec{}, errors.New("AZURE_FILES_E2E_CALIBRATE=1 only records the probe marker of the smb-late-<age> cells; " +
			"select one of them in AZURE_FILES_E2E_CELLS, or unset it")
	}
	if opts.forceLoss && !forced {
		return runSpec{}, errors.New("AZURE_FILES_E2E_FORCE_LOSS=1 only applies to the loss-accounted cells, smb-gzip and smb-delete-recreate; " +
			"select one of them in AZURE_FILES_E2E_CELLS, or unset it")
	}
	return spec, nil
}

// parseNodes reads the AKS node count. More than one node runs as many
// Agents, which each collect every record, so only the multi-node scenario,
// which expects that, may ask for them, and it needs two at least.
func parseNodes(value string, scenario scenarioOptions) (int, error) {
	fallback := defaultNodes
	if scenario.kind == multiNodeScenario {
		fallback = defaultMultiNodeNodes
	}
	nodes, err := parseBoundedInt("node count", value, fallback, 1, maxNodes)
	switch {
	case err != nil:
		return 0, err
	case scenario.kind == multiNodeScenario && nodes < 2:
		return 0, fmt.Errorf("the %s scenario needs at least 2 nodes, not %d", multiNodeScenario, nodes)
	case scenario.kind != multiNodeScenario && nodes > 1:
		return 0, fmt.Errorf("%d nodes run %d Agents, which would each collect every record of every cell; only the %s scenario runs more than one node", nodes, nodes, multiNodeScenario)
	}
	return nodes, nil
}

// windowsCell returns the provisioned cell that the Windows file server
// serves, if the run has one.
func (spec runSpec) windowsCell() (cell, bool) {
	for _, c := range spec.cells {
		if c.onWindows() {
			return c, true
		}
	}
	return cell{}, false
}

// setWindowsServerHost gives the Windows file server's address to the cells
// it serves, once the storage pass has created it.
func (spec *runSpec) setWindowsServerHost(host string) {
	for i := range spec.cells {
		if spec.cells[i].onWindows() {
			spec.cells[i].serverHost = host
		}
	}
}

// writerOptions applies the cell's fixed rotation mode and its defaults to the
// run's writer options. A run that sets another rotation mode than a cell's
// fixed one is refused rather than ignored.
func (c cell) writerOptions(run writerOptions, set writerOptionsSet) (writerOptions, error) {
	w := run
	if c.fixedMode != "" {
		if set.mode && run.mode != c.fixedMode {
			return writerOptions{}, fmt.Errorf("cell %s always rotates by %s; unset the rotation mode or select another cell", c.name, c.fixedMode)
		}
		w.mode = c.fixedMode
	}
	w = w.withDefaults(set, c.writerDefaults)
	if err := w.validate(); err != nil {
		return writerOptions{}, fmt.Errorf("cell %s: %w", c.name, err)
	}
	return w, nil
}

// Every SMB cell but smb itself is opt-in: it runs only when
// AZURE_FILES_E2E_CELLS names it, so the default matrix stays the four cells
// it always was.
const (
	// smbModeCellPrefix names the cells that fix the SMB source's rotation
	// mode, smb-<mode>.
	smbModeCellPrefix = "smb-"
	globLoadCellName  = "smb-glob-load"
	// The glob-load cell writes this many services, svc-1 to svc-<N>, under
	// one smb source, with this period and total rate, unless the run sets
	// them.
	globLoadStreams         = 8
	globLoadPeriodMs        = 10000
	globLoadRateBytesPerSec = 2 * 1000 * 1000
)

// allCells lists every cell. Storage accounts follow the stack, so a reused
// stack keeps them. Shares follow the run, so every run starts from empty
// shares: no ledger is left over, and the SMB source's registry entries, which
// name the share, cannot match an earlier run.
func allCells(stackName, runID string) []cell {
	stackDigest := hexDigest(stackName)
	runDigest := hexDigest(runID)
	lineFingerprint := fingerprintConfig{strategy: "line_checksum", count: 1, maxBytes: 4096}
	// accountPrefix must leave 14 characters of the 24 an account name has.
	smbCell := func(name, accountPrefix, sharePrefix string) cell {
		return cell{
			name:         name,
			reader:       smbReader,
			mountOptions: mountOptionsActimeo1,
			accountName:  accountPrefix + stackDigest[:14],
			shareName:    sharePrefix + "-" + runDigest[:12],
		}
	}

	all := []cell{
		{
			name:            "file-line",
			reader:          fileReader,
			fingerprint:     lineFingerprint,
			mountOptions:    mountOptionsActimeo1,
			accountName:     "ddafline" + stackDigest[:14],
			shareName:       "fline-" + runDigest[:12],
			inDefaultMatrix: true,
		},
		{
			name:            "file-byte",
			reader:          fileReader,
			fingerprint:     fingerprintConfig{strategy: "byte_checksum", count: 2048, maxBytes: 2048},
			mountOptions:    mountOptionsActimeo1,
			accountName:     "ddafbyte" + stackDigest[:14],
			shareName:       "fbyte-" + runDigest[:12],
			inDefaultMatrix: true,
		},
		{
			name:            "file-line-actimeo30",
			reader:          fileReader,
			fingerprint:     lineFingerprint,
			mountOptions:    mountOptionsActimeo30,
			accountName:     "ddafln30" + stackDigest[:14],
			shareName:       "fline30-" + runDigest[:12],
			inDefaultMatrix: true,
		},
	}
	smb := smbCell("smb", "ddafsmb", "smb")
	smb.inDefaultMatrix = true
	all = append(all, smb)

	// The SMB source under each rotation mode but rename, which the smb cell
	// covers. Each has its own share and writer, so one run covers them all.
	for _, mode := range []struct {
		mode                       rotationMode
		accountPrefix, sharePrefix string
	}{
		{copyTruncateRotation, "ddafsmbct", "smbct"},
		{deleteRecreateRotation, "ddafsmbdr", "smbdr"},
		{gzipRotation, "ddafsmbgz", "smbgz"},
	} {
		c := smbCell(smbModeCellPrefix+string(mode.mode), mode.accountPrefix, mode.sharePrefix)
		c.fixedMode = mode.mode
		all = append(all, c)
	}

	// One cell per late-append age: a collected early marker keeps the
	// drain open, so a cell can only probe one age.
	for i := range lateMarkerProbes {
		probe := lateMarkerProbes[i]
		c := smbCell(lateMarkerCellName(probe), "ddafsl"+strconv.Itoa(probe.ageMs), "smbl"+strconv.Itoa(probe.ageMs))
		c.fixedMode = renameRotation
		c.probe = &probe
		all = append(all, c)
	}

	globLoad := smbCell(globLoadCellName, "ddafsmbgl", "smbgl")
	globLoad.writerDefaults = writerDefaults{
		periodMs:        globLoadPeriodMs,
		rateBytesPerSec: globLoadRateBytesPerSec,
		streams:         globLoadStreams,
	}
	all = append(all, globLoad)

	// The SMB source against a Windows Server share that requires signing,
	// rather than Azure Files. It has no storage account: its share is on the
	// run's Windows VM (see windows.go).
	all = append(all, cell{
		name:      windowsCellName,
		reader:    smbReader,
		server:    windowsFileServer,
		shareName: "smbwin-" + runDigest[:12],
	})

	for i := range all {
		if all[i].server == "" {
			all[i].server = azureFilesServer
		}
		all[i].service = "azure-files-" + all[i].name
		all[i].volumeName = "azure-files-" + all[i].name
		all[i].writerName = "writer-" + all[i].name
	}
	return all
}

// resolveStackName returns the reused stack name when one is given, and a
// stack of this run's own otherwise. A run with more than one node gets a
// stack of its own, with the node count as a suffix, so that it never resizes
// the cluster of a reused one-node stack: AZURE_FILES_E2E_STACK=dev runs on
// dev with one node and on dev-n2 with two.
func resolveStackName(override, runID string, nodes int) (string, error) {
	suffix := ""
	if nodes > 1 {
		suffix = "-n" + strconv.Itoa(nodes)
	}
	override = strings.TrimSpace(override)
	if override == "" {
		return "azure-files-" + hexDigest(runID)[:8] + suffix, nil
	}
	if !stackNamePattern.MatchString(override) {
		return "", fmt.Errorf("stack name %q must be 1 to 40 lowercase letters, digits or inner hyphens", override)
	}
	if nodeSuffixPattern.MatchString(override) {
		return "", fmt.Errorf("stack name %q ends like the stack of a multi-node run, whose -n<nodes> suffix the suite adds itself; name the one-node stack and set the node count instead", override)
	}
	return override + suffix, nil
}

func hexDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// filterCells keeps only the named cells, so a run can provision one cell
// instead of the whole matrix. An empty filter keeps the default matrix.
func filterCells(all []cell, cellFilter string) ([]cell, error) {
	wanted := make([]string, 0, len(all))
	for _, name := range strings.Split(cellFilter, ",") {
		if name = strings.TrimSpace(name); name != "" {
			wanted = append(wanted, name)
		}
	}
	if len(wanted) == 0 {
		var matrix []cell
		for _, c := range all {
			if c.inDefaultMatrix {
				matrix = append(matrix, c)
			}
		}
		return matrix, nil
	}

	selected := make([]cell, 0, len(wanted))
	for _, name := range wanted {
		found := false
		for _, c := range all {
			if c.name == name {
				selected = append(selected, c)
				found = true
				break
			}
		}
		if !found {
			names := make([]string, 0, len(all))
			for _, c := range all {
				names = append(names, c.name)
			}
			return nil, fmt.Errorf("unknown cell %q; known cells are %s", name, strings.Join(names, ","))
		}
	}
	return selected, nil
}

// forceLoss reports whether the run forces the losses of its loss-accounted
// cells (AZURE_FILES_E2E_FORCE_LOSS).
func (spec runSpec) forceLoss() bool {
	for _, c := range spec.cells {
		if c.writer.forceLoss {
			return true
		}
	}
	return false
}

func (spec runSpec) hasReader(reader readerKind) bool {
	for _, c := range spec.cells {
		if c.reader == reader {
			return true
		}
	}
	return false
}

// cellNamed returns the provisioned cell of that name.
func (spec runSpec) cellNamed(name string) (cell, bool) {
	for _, c := range spec.cells {
		if c.name == name {
			return c, true
		}
	}
	return cell{}, false
}

// fakeintakeOptions must be identical in both passes: the passes update one
// stack, and any difference would replace the Fakeintake VM between them.
func fakeintakeOptions() azurekubernetes.ProvisionerOption {
	return azurekubernetes.WithFakeIntakeOptions(
		fakeintake.WithRetentionPeriod(fakeintakeRetention),
		fakeintake.WithoutDDDevForwarding(),
	)
}

// azureFilesEnv is the suite's environment: the AKS cluster with its Agent
// and Fakeintake, and the Windows file server of the smb-windows cell, a VM
// on the cluster's subnet.
type azureFilesEnv struct {
	environments.Kubernetes
	// WindowsServer is nil unless the run provisions the smb-windows cell.
	WindowsServer *components.RemoteHost
}

const (
	provisionerName = "azurefiles"
	// provisionerID is the ID azurekubernetes.AKSProvisioner gave this
	// suite's provisioner before it needed an environment of its own. Both
	// passes must use the same ID: UpdateEnv destroys the stack of a
	// provisioner whose ID is not in the new set.
	provisionerID = "azure-aks" + provisionerName
)

// storageProvisioner creates AKS and the Azure Files resources without an
// Agent. The second UpdateEnv pass installs the Agent only after the CSI
// secrets and shares already exist on the stack.
func (spec runSpec) storageProvisioner() provisioners.TypedProvisioner[azureFilesEnv] {
	return spec.provisioner(
		// Called without arguments, this sets the Agent options to nil, which
		// makes the provisioner skip the Agent. Its default, an empty non-nil
		// list, would install one.
		azurekubernetes.WithAgentOptions(),
		azurekubernetes.WithWorkloadApp(spec.storageWorkload),
	)
}

func (spec runSpec) agentProvisioner() provisioners.TypedProvisioner[azureFilesEnv] {
	return spec.provisioner(
		azurekubernetes.WithAgentOptions(
			kubernetesagentparams.WithHelmValues(spec.agentHelmValues()),
			kubernetesagentparams.WithoutLogsContainerCollectAll(),
		),
		azurekubernetes.WithAgentDependentWorkloadApp(spec.writerWorkload),
		azurekubernetes.WithWorkloadApp(spec.storageWorkload),
	)
}

// aksOptions are the provisioner options both passes share. They must be
// identical in both: the passes update one stack, and any difference would
// replace the cluster or the Fakeintake VM between them.
func (spec runSpec) aksOptions() []azurekubernetes.ProvisionerOption {
	return []azurekubernetes.ProvisionerOption{
		azurekubernetes.WithName(provisionerName),
		fakeintakeOptions(),
		azurekubernetes.WithAKSOptions(aks.WithNodeCount(spec.nodes)),
	}
}

// provisioner runs the AKS provisioner in an Azure environment of its own, in
// which it also creates the Windows file server when the run has the
// smb-windows cell, on the same subnet as the AKS nodes and the Fakeintake.
func (spec runSpec) provisioner(opts ...azurekubernetes.ProvisionerOption) provisioners.TypedProvisioner[azureFilesEnv] {
	return provisioners.NewTypedPulumiProvisioner(provisionerID, func(ctx *pulumi.Context, env *azureFilesEnv) error {
		azureEnv, err := azureresources.NewEnvironment(ctx)
		if err != nil {
			return err
		}
		if _, ok := spec.windowsCell(); ok {
			if err := newWindowsServer(azureEnv, env); err != nil {
				return err
			}
		} else {
			// The suite creates every component of the environment, so one
			// this run does not provision is set to nil.
			env.WindowsServer = nil
		}
		params := azurekubernetes.GetProvisionerParams(append(spec.aksOptions(), opts...)...)
		return azurekubernetes.AKSRunWithEnv(ctx, azureEnv, &env.Kubernetes, params)
	}, nil)
}

type azureStorageAccount struct {
	pulumi.CustomResourceState
	Name pulumi.StringOutput `pulumi:"name"`
}

type azureFileShare struct {
	pulumi.CustomResourceState
}

// storageWorkload creates one storage account and file share per cell through
// the generic azure-native resource tokens, so the suite needs no storage SDK
// module.
//
// The account keys only ever leave Azure as Pulumi secrets inside Kubernetes
// Secrets: one per cell in the e2e namespace for the CSI driver, and, for SMB
// cells, a copy in the Agent namespace that the Agent pod mounts as a file.
// Neither is a stack output.
func (spec runSpec) storageWorkload(env config.Env, kubeProvider *kubernetes.Provider) (*kubecomp.Workload, error) {
	azureEnv, ok := env.(*azureresources.Environment)
	if !ok {
		return nil, fmt.Errorf("Azure Files workload requires *azure.Environment, got %T", env)
	}

	ctx := env.Ctx()
	workload := &kubecomp.Workload{}
	if err := ctx.RegisterComponentResource("dd:e2e:AzureFilesStorage", spec.runID, workload); err != nil {
		return nil, err
	}

	kubeOpts := []pulumi.ResourceOption{
		pulumi.Provider(kubeProvider),
		pulumi.Parent(workload),
		pulumi.DeletedWith(kubeProvider),
	}
	namespace, err := corev1.NewNamespace(ctx, e2eNamespace, &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String(e2eNamespace),
			Labels: pulumi.StringMap{
				partOfLabel: pulumi.String(partOfLabelValue),
				runIDLabel:  pulumi.String(spec.runID),
			},
		},
	}, kubeOpts...)
	if err != nil {
		return nil, err
	}
	agentSecretOpts := append([]pulumi.ResourceOption{}, kubeOpts...)
	kubeOpts = append(kubeOpts, utils.PulumiDependsOn(namespace))

	if env.ImagePullRegistry() != "" {
		if _, err := utils.NewImagePullSecret(env, e2eNamespace, kubeOpts...); err != nil {
			return nil, err
		}
	}

	// The Agent is installed by the second pass, but the Secrets it mounts
	// must exist before its pod starts. A patch rather than a Namespace
	// leaves the namespace shared with the Agent installation, which
	// patches it too.
	agentNS, err := corev1.NewNamespacePatch(ctx, "azure-files-agent-namespace", &corev1.NamespacePatchArgs{
		Metadata: &metav1.ObjectMetaPatchArgs{
			Name: pulumi.String(agentNamespace),
		},
	}, agentSecretOpts...)
	if err != nil {
		return nil, err
	}
	agentSecretOpts = append(agentSecretOpts, utils.PulumiDependsOn(agentNS))

	// The ARM ID of every storage account, so the test process can address
	// one with the Azure CLI (the key-rotation scenario renews a key). It
	// names the subscription and resource group the accounts were created in.
	accountIDs := pulumi.StringMap{}
	var accounts []pulumi.Resource
	for _, c := range spec.cells {
		if c.onWindows() {
			// No storage account: the share is on the Windows VM, and the
			// Secrets hold the password of its local reader account.
			if err := newWindowsReaderSecrets(azureEnv, workload, c, kubeOpts, agentSecretOpts); err != nil {
				return nil, err
			}
			continue
		}
		account := &azureStorageAccount{}
		err := ctx.RegisterResource("azure-native:storage:StorageAccount", c.accountName, pulumi.Map{
			"accountName":       pulumi.String(c.accountName),
			"kind":              pulumi.String("StorageV2"),
			"resourceGroupName": pulumi.String(azureEnv.DefaultResourceGroup()),
			"sku":               pulumi.Map{"name": pulumi.String("Standard_LRS")},
			"tags":              env.ResourcesTags(),
		}, account, pulumi.Parent(workload), azureEnv.WithProviders(config.ProviderAzure))
		if err != nil {
			return nil, err
		}
		accountIDs[c.name] = account.ID().ToStringOutput()
		accounts = append(accounts, account)

		share := &azureFileShare{}
		err = ctx.RegisterResource("azure-native:storage:FileShare", c.shareName, pulumi.Map{
			"accountName":       account.Name,
			"resourceGroupName": pulumi.String(azureEnv.DefaultResourceGroup()),
			"shareName":         pulumi.String(c.shareName),
			"shareQuota":        pulumi.Int(shareQuotaGiB),
		}, share, pulumi.Parent(workload), azureEnv.WithProviders(config.ProviderAzure), pulumi.DependsOn([]pulumi.Resource{account}))
		if err != nil {
			return nil, err
		}

		keys := ctx.InvokeOutput(
			"azure-native:storage:listStorageAccountKeys",
			pulumi.Map{
				"accountName":       account.Name,
				"resourceGroupName": pulumi.String(azureEnv.DefaultResourceGroup()),
			},
			pulumi.MapOutput{},
			pulumi.InvokeOutputOptions{InvokeOptions: []pulumi.InvokeOption{
				azureEnv.WithProvider(config.ProviderAzure),
				pulumi.DependsOn([]pulumi.Resource{account}),
			}},
		).(pulumi.MapOutput)
		listedKeys := keys.MapIndex(pulumi.String("keys"))
		rawAccountKey := listedKeys.ApplyT(firstStorageAccountKey).(pulumi.StringOutput)
		accountKey := pulumi.ToSecret(rawAccountKey).(pulumi.StringOutput)
		// The Agent's copy holds key1 like the writer's, except in the
		// key-rotation scenario, whose Agent reads with key2.
		agentAccountKey := accountKey
		if c.agentKeyIndex != 0 {
			index := c.agentKeyIndex
			rawAgentKey := listedKeys.ApplyT(func(value any) (string, error) {
				return storageAccountKeyAt(value, index)
			}).(pulumi.StringOutput)
			agentAccountKey = pulumi.ToSecret(rawAgentKey).(pulumi.StringOutput)
		}
		storageSecretOpts := append([]pulumi.ResourceOption{}, kubeOpts...)
		storageSecretOpts = append(storageSecretOpts, utils.PulumiDependsOn(share))
		_, err = corev1.NewSecret(ctx, c.secretName(), &corev1.SecretArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String(c.secretName()),
				Namespace: pulumi.String(e2eNamespace),
			},
			StringData: pulumi.StringMap{
				accountNameSecretKey: account.Name,
				accountKeySecretKey:  accountKey,
			},
		}, storageSecretOpts...)
		if err != nil {
			return nil, err
		}

		// The Agent pod mounts either the share (a CSI volume) or the key (a
		// Secret volume). Both read the Secret from the pod's own namespace;
		// the CSI driver ignores secretNamespace for inline volumes. So the
		// Secret is copied next to the Agent.
		_, err = corev1.NewSecret(ctx, c.secretName()+"-agent", &corev1.SecretArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String(c.secretName()),
				Namespace: pulumi.String(agentNamespace),
				Labels: pulumi.StringMap{
					partOfLabel: pulumi.String(partOfLabelValue),
					cellLabel:   pulumi.String(c.name),
				},
			},
			StringData: pulumi.StringMap{
				accountNameSecretKey: account.Name,
				accountKeySecretKey:  agentAccountKey,
			},
		}, agentSecretOpts...)
		if err != nil {
			return nil, err
		}
	}

	accountOpts := append([]pulumi.ResourceOption{}, kubeOpts...)
	accountOpts = append(accountOpts, pulumi.DependsOn(accounts))
	if _, err := corev1.NewConfigMap(ctx, storageAccountsConfigMapName, &corev1.ConfigMapArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.String(storageAccountsConfigMapName),
			Namespace: pulumi.String(e2eNamespace),
			Labels: pulumi.StringMap{
				partOfLabel: pulumi.String(partOfLabelValue),
				runIDLabel:  pulumi.String(spec.runID),
			},
		},
		Data: accountIDs,
	}, accountOpts...); err != nil {
		return nil, err
	}

	if err := ctx.RegisterResourceOutputs(workload, pulumi.Map{
		"namespace": pulumi.String(e2eNamespace),
		"runID":     pulumi.String(spec.runID),
	}); err != nil {
		return nil, err
	}
	return workload, nil
}

func firstStorageAccountKey(value any) (string, error) {
	return storageAccountKeyAt(value, 0)
}

// storageAccountKeyAt returns key1 (index 0) or key2 (index 1) of a
// listStorageAccountKeys result. Errors never contain a key.
func storageAccountKeyAt(value any, index int) (string, error) {
	keys, ok := value.([]any)
	if !ok || len(keys) == 0 {
		return "", errors.New("Azure returned no storage account keys")
	}
	if index < 0 || index >= len(keys) {
		return "", fmt.Errorf("Azure returned %d storage account keys, not key%d", len(keys), index+1)
	}
	key, ok := keys[index].(map[string]any)
	if !ok {
		return "", fmt.Errorf("Azure returned an unexpected storage account key type %T", keys[index])
	}
	keyValue, ok := key["value"].(string)
	if !ok || keyValue == "" {
		return "", errors.New("Azure returned an empty storage account key")
	}
	return keyValue, nil
}

func (spec runSpec) writerWorkload(
	env config.Env,
	kubeProvider *kubernetes.Provider,
	dependsOnAgent pulumi.ResourceOption,
) (*kubecomp.Workload, error) {
	ctx := env.Ctx()
	workload := &kubecomp.Workload{}
	if err := ctx.RegisterComponentResource("dd:e2e:AzureFilesWriters", spec.runID, workload); err != nil {
		return nil, err
	}

	kubeOpts := []pulumi.ResourceOption{
		pulumi.Provider(kubeProvider),
		pulumi.Parent(workload),
		pulumi.DeletedWith(kubeProvider),
		dependsOnAgent,
	}
	runtime := spec.workloadRuntime(env.InternalDockerhubMirror())
	if len(runtime.scripts) > 0 {
		configMap, err := corev1.NewConfigMap(ctx, workloadConfigMapName, &corev1.ConfigMapArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String(workloadConfigMapName),
				Namespace: pulumi.String(e2eNamespace),
				Labels: pulumi.StringMap{
					partOfLabel: pulumi.String(partOfLabelValue),
					runIDLabel:  pulumi.String(spec.runID),
				},
			},
			Data: pulumi.ToStringMap(runtime.scripts),
		}, kubeOpts...)
		if err != nil {
			return nil, err
		}
		kubeOpts = append(kubeOpts, utils.PulumiDependsOn(configMap))
	}
	for _, c := range spec.cells {
		if c.onWindows() {
			// Its writer runs on the Windows VM, started by the test.
			continue
		}
		if _, err := spec.newWriterDeployment(env, c, runtime, kubeOpts); err != nil {
			return nil, err
		}
	}

	if err := ctx.RegisterResourceOutputs(workload, pulumi.Map{
		"namespace": pulumi.String(e2eNamespace),
		"runID":     pulumi.String(spec.runID),
	}); err != nil {
		return nil, err
	}
	return workload, nil
}

// writerRunID identifies one cell of one run in the writer's records, its
// ledger, and its markers.
func (spec runSpec) writerRunID(c cell) string {
	return spec.runID + "-" + c.name
}

// streamRunIDs are the run IDs of a cell's streams, in their records, ledgers
// and markers: the cell's run ID for a single app.log, and <run ID>-svc-<i>
// for each stream of a multi-stream writer, as logwriter.py and appender.sh
// derive them.
func (spec runSpec) streamRunIDs(c cell) []string {
	if c.writer.streams == 0 {
		return []string{spec.writerRunID(c)}
	}
	ids := make([]string, 0, c.writer.streams)
	for _, dir := range c.writer.streamDirs() {
		ids = append(ids, spec.writerRunID(c)+"-"+dir)
	}
	return ids
}

// writerEnv is the writer configuration that both writers read, followed by
// the options only the Python writer has, when they are not the defaults.
func (spec runSpec) writerEnv(c cell) []envVar {
	vars := []envVar{
		{"LOGWRITER_LOG_DIR", logMountPath},
		{"LOGWRITER_RUN_ID", spec.writerRunID(c)},
		{"LOGWRITER_TARGET_BYTES_SEQUENCE", writerTargetSequence},
		{"LOGWRITER_HEAD_PAUSE_MS", strconv.Itoa(c.writer.headPauseMs())},
		{"LOGWRITER_MAX_RECORDS_PER_PERIOD", "5000"},
		{"TZ", "UTC"},
	}
	vars = append(vars, c.writer.writerEnv()...)
	if atRisk := c.copyTruncateAtRiskMs(); c.writer.mode == copyTruncateRotation && atRisk < copyTruncateHoldMs {
		vars = append(vars, envVar{"LOGWRITER_COPYTRUNCATE_AT_RISK_MS", strconv.Itoa(atRisk)})
	}
	return vars
}

// copyTruncateAtRiskMs is how long before a copytruncate's truncation a record
// must have been written for the cell's reader to be allowed to lose it: the
// whole hold for a file source, which reads through the mount's attribute
// cache, and copyTruncateSMBAtRiskMs for an SMB source, which polls app.log.
func (c cell) copyTruncateAtRiskMs() int {
	if c.reader == smbReader {
		return copyTruncateSMBAtRiskMs
	}
	return copyTruncateHoldMs
}

func toEnvVarArray(vars []envVar) corev1.EnvVarArray {
	array := make(corev1.EnvVarArray, 0, len(vars))
	for _, v := range vars {
		array = append(array, corev1.EnvVarArgs{Name: pulumi.String(v.name), Value: pulumi.String(v.value)})
	}
	return array
}

func (spec runSpec) newWriterDeployment(
	env config.Env,
	c cell,
	runtime workloadRuntime,
	baseOpts []pulumi.ResourceOption,
) (*appsv1.Deployment, error) {
	labels := pulumi.StringMap{
		"app.kubernetes.io/name": pulumi.String(c.writerName),
		partOfLabel:              pulumi.String(partOfLabelValue),
		runIDLabel:               pulumi.String(spec.runID),
		cellLabel:                pulumi.String(c.name),
	}
	templateMetadata := metav1.ObjectMetaArgs{Labels: labels}
	if len(runtime.scripts) > 0 {
		templateMetadata.Annotations = pulumi.StringMap{
			workloadChecksumAnnotation: pulumi.String(runtime.scriptsChecksum()),
		}
	}

	opts := append([]pulumi.ResourceOption{}, baseOpts...)
	return appsv1.NewDeployment(env.Ctx(), c.writerName, &appsv1.DeploymentArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.String(c.writerName),
			Namespace: pulumi.String(e2eNamespace),
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: metav1.LabelSelectorArgs{MatchLabels: labels},
			Strategy: appsv1.DeploymentStrategyArgs{Type: pulumi.String("Recreate")},
			Template: corev1.PodTemplateSpecArgs{
				Metadata: templateMetadata,
				Spec:     spec.writerPodSpec(c, runtime, env.ImagePullRegistry() != ""),
			},
		},
	}, opts...)
}

func (spec runSpec) ledgerEnv(c cell, runtime workloadRuntime) []envVar {
	vars := append([]envVar{{"LOGWRITER_LOG_DIR", logMountPath}}, runtime.ledgerEnv...)
	return append(vars, c.writer.streamEnv()...)
}

// appenderEnv configures the appender. Its journal path only applies to a
// single app.log; each stream directory has a journal of its own.
func (spec runSpec) appenderEnv(c cell) []envVar {
	vars := []envVar{
		{"LOGWRITER_LOG_DIR", logMountPath},
		{"LOGWRITER_RUN_ID", spec.writerRunID(c)},
		{"LOGWRITER_MARKER_JOURNAL_PATH", logMountPath + "/" + postRotationMarkerJournalName},
		{"LOGWRITER_APPEND_DELAYS_MS", c.markers.appenderValue()},
		{"LOGWRITER_APPEND_POLL_MS", strconv.Itoa(postRotationMarkerPollMs)},
		{"TZ", "UTC"},
	}
	return append(vars, c.writer.streamEnv()...)
}

// writerPodSpec runs a cell's writer, ledger and appender on its share.
func (spec runSpec) writerPodSpec(c cell, runtime workloadRuntime, imagePullSecret bool) *corev1.PodSpecArgs {
	volume := corev1.VolumeArgs{
		Name: pulumi.String(c.volumeName),
		Csi: corev1.CSIVolumeSourceArgs{
			Driver:   pulumi.String("file.csi.azure.com"),
			ReadOnly: pulumi.Bool(false),
			VolumeAttributes: pulumi.StringMap{
				"storageAccount":  pulumi.String(c.accountName),
				"server":          pulumi.String(c.host()),
				"shareName":       pulumi.String(c.shareName),
				"secretName":      pulumi.String(c.secretName()),
				"secretNamespace": pulumi.String(e2eNamespace),
				"mountOptions":    pulumi.String(c.mountOptions),
			},
		},
	}
	volumeMount := corev1.VolumeMountArgs{
		Name:      pulumi.String(c.volumeName),
		MountPath: pulumi.String(logMountPath),
	}
	volumes := corev1.VolumeArray{volume}
	volumeMounts := corev1.VolumeMountArray{volumeMount}
	if len(runtime.scripts) > 0 {
		volumes = append(volumes, corev1.VolumeArgs{
			Name: pulumi.String(workloadVolumeName),
			ConfigMap: corev1.ConfigMapVolumeSourceArgs{
				Name:        pulumi.String(workloadConfigMapName),
				DefaultMode: pulumi.Int(0o555),
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMountArgs{
			Name:      pulumi.String(workloadVolumeName),
			MountPath: pulumi.String(workloadDir),
			ReadOnly:  pulumi.Bool(true),
		})
	}

	writer := corev1.ContainerArgs{
		Name:            pulumi.String(writerContainerName),
		Image:           pulumi.String(runtime.image),
		ImagePullPolicy: pulumi.String("IfNotPresent"),
		Env:             toEnvVarArray(append(spec.writerEnv(c), runtime.writerEnv...)),
		VolumeMounts:    volumeMounts,
		ReadinessProbe: corev1.ProbeArgs{
			Exec: corev1.ExecActionArgs{Command: pulumi.ToStringArray([]string{
				"/bin/sh", "-c", fmt.Sprintf("test $(wc -c < %s) -ge 4096", c.writer.sharePaths(activeLogName)[0]),
			})},
			PeriodSeconds:    pulumi.Int(2),
			FailureThreshold: pulumi.Int(45),
		},
	}
	if runtime.writerCommand != nil {
		writer.Command = pulumi.ToStringArray(runtime.writerCommand)
	}
	podSpec := &corev1.PodSpecArgs{
		NodeSelector: pulumi.StringMap{
			"kubernetes.io/arch": pulumi.String("amd64"),
			"kubernetes.io/os":   pulumi.String("linux"),
		},
		Containers: corev1.ContainerArray{
			writer,
			corev1.ContainerArgs{
				Name:            pulumi.String(ledgerContainerName),
				Image:           pulumi.String(runtime.image),
				ImagePullPolicy: pulumi.String("IfNotPresent"),
				Command:         pulumi.ToStringArray(runtime.ledgerCommand),
				Env:             toEnvVarArray(spec.ledgerEnv(c, runtime)),
				VolumeMounts:    volumeMounts,
			},
			// The appender shares the writer pod, so it appends through the
			// same CIFS mount that performed the rename. The rotation is
			// therefore visible to it immediately, whatever actimeo the cell
			// uses for revalidating remotely changed metadata.
			corev1.ContainerArgs{
				Name:            pulumi.String(appenderContainerName),
				Image:           pulumi.String(runtime.image),
				ImagePullPolicy: pulumi.String("IfNotPresent"),
				Command:         pulumi.ToStringArray(runtime.appenderCommand),
				Env:             toEnvVarArray(spec.appenderEnv(c)),
				VolumeMounts:    volumeMounts,
			},
		},
		Volumes: volumes,
	}
	if runtime.kind == stockWorkload {
		podSpec.SecurityContext = corev1.PodSecurityContextArgs{
			RunAsNonRoot: pulumi.Bool(true),
			RunAsUser:    pulumi.Int(stockWorkloadUser),
			RunAsGroup:   pulumi.Int(stockWorkloadUser),
		}
	}
	if imagePullSecret {
		podSpec.ImagePullSecrets = corev1.LocalObjectReferenceArray{
			corev1.LocalObjectReferenceArgs{Name: pulumi.String(utils.DefaultImagePullSecretName)},
		}
	}
	return podSpec
}

// agentMountPath is where the Agent mounts the share of a file reader cell.
func agentMountPath(c cell) string {
	return logMountPath + "/" + c.name
}

// yamlPattern quotes a path pattern that YAML would otherwise read as an alias.
func yamlPattern(pattern string) string {
	if strings.HasPrefix(pattern, "*") {
		return strconv.Quote(pattern)
	}
	return pattern
}

// smbPasswordHandle is the secret backend handle the SMB source resolves to
// the cell's storage account key. The key reaches the Agent pod only as a file
// of its mounted Secret; /readsecret_multiple_providers.sh reads that file.
func smbPasswordHandle(c cell) string {
	return fmt.Sprintf("ENC[file@%s/%s]", agentKeyDir(c), accountKeySecretKey)
}

func (spec runSpec) agentHelmValues() string {
	var sources strings.Builder
	var volumes strings.Builder
	var mounts strings.Builder
	// One source per cell reads every stream of its share. The rotated names
	// never match the active file's pattern.
	for _, c := range spec.cells {
		pattern := c.writer.activeLogPattern()
		switch c.reader {
		case fileReader:
			fmt.Fprintf(&sources, `      - type: file
        path: %[1]s/%[2]s
        exclude_paths:
          - %[1]s/%[2]s.*
        service: %[3]s
        source: java
        start_position: beginning
        fingerprint_config:
          fingerprint_strategy: %[4]s
          count: %[5]d
          count_to_skip: 0
          max_bytes: %[6]d
`, agentMountPath(c), pattern, c.service, c.fingerprint.strategy, c.fingerprint.count, c.fingerprint.maxBytes)
			fmt.Fprintf(&volumes, `    - name: %s
      csi:
        driver: file.csi.azure.com
        readOnly: true
        volumeAttributes:
          storageAccount: %s
          server: %s
          shareName: %s
          secretName: %s
          secretNamespace: %s
          mountOptions: %q
`, c.volumeName, c.accountName, c.host(), c.shareName, c.secretName(), agentNamespace, c.mountOptions)
			fmt.Fprintf(&mounts, `    - name: %s
      mountPath: %s
      readOnly: true
`, c.volumeName, agentMountPath(c))
		case smbReader:
			// The path is relative to the share root. Rotated names do not
			// match it: the source has to follow the rotated file by its
			// server FileId to drain it. A pattern starting with * must be
			// quoted in YAML.
			fmt.Fprintf(&sources, `      - type: smb
        path: %s
        service: %s
        source: java
        start_position: beginning
        smb:
          host: %s
          share: %s
          username: %s
          password: %q
          poll_interval: %d
`, yamlPattern(pattern), c.service, c.host(), c.shareName, c.smbUsername(), smbPasswordHandle(c), smbPollIntervalSeconds)
			// The chart mounts agents.volumeMounts into every container of
			// the Agent pod; only the core Agent resolves the handle. The
			// Agent runs as root, so 0400 (256) still lets it read the key.
			fmt.Fprintf(&volumes, `    - name: %s
      secret:
        secretName: %s
        defaultMode: 256
        items:
          - key: %s
            path: %s
`, c.agentKeyVolumeName(), c.secretName(), accountKeySecretKey, accountKeySecretKey)
			fmt.Fprintf(&mounts, `    - name: %s
      mountPath: %s
      readOnly: true
`, c.agentKeyVolumeName(), agentKeyDir(c))
		}
		fmt.Fprintf(&sources, `        tags:
          - e2e_run_id:%s
          - e2e_cell:%s
          - e2e_profile:%s
`, spec.runID, c.name, spec.profile.name)
	}

	var values strings.Builder
	values.WriteString(`datadog:
  logs:
    enabled: true
    containerCollectAll: false
  logLevel: DEBUG
`)
	if spec.hasReader(smbReader) {
		// The handle names a mounted file, so the Agent needs no permission
		// to read Secrets through the API. Without
		// enableGlobalPermissions: false the chart would grant it every
		// Secret in the cluster.
		values.WriteString(`  secretBackend:
    command: /readsecret_multiple_providers.sh
    enableGlobalPermissions: false
`)
	}
	fmt.Fprintf(&values, `  confd:
    azure_files.yaml: |-
      logs:
%s`, sources.String())
	// The settings go to the core Agent container as a map. A datadog.env list
	// would replace the framework's own, which points the Agent at Fakeintake:
	// Helm replaces lists from later values files instead of merging them.
	fmt.Fprintf(&values, `agents:
  containers:
    agent:
      envDict:
        DD_LOGS_CONFIG_FILE_SCAN_PERIOD: "%d"
        DD_LOGS_CONFIG_CLOSE_TIMEOUT: "%d"
        DD_LOGS_CONFIG_UNRELIABLE_MOUNT_ENABLED: "%t"
`, fileScanPeriodSeconds, closeTimeoutSeconds, spec.profile.unreliableMount)
	if spec.scenario.kind == keyRotationScenario {
		// secret_refresh_interval and secret_refresh_scatter, read in
		// pkg/config/setup/config.go: the core Agent runs the secret backend
		// again every interval and, when the smb password changed, schedules
		// the azure_files configuration again with it. Without scatter the
		// first refresh comes one interval after start-up rather than at a
		// random time within it. The chart's
		// datadog.secretBackend.refreshInterval would set the interval in
		// every container; only the core Agent resolves the smb handle.
		fmt.Fprintf(&values, `        DD_SECRET_REFRESH_INTERVAL: "%d"
        DD_SECRET_REFRESH_SCATTER: "false"
`, secretRefreshIntervalSeconds)
	}
	if volumes.Len() > 0 {
		fmt.Fprintf(&values, `  volumes:
%s  volumeMounts:
%s`, volumes.String(), mounts.String())
	}
	return values.String()
}
