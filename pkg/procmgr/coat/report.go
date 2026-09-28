// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// SupportReport is a point-in-time dump of dd-procmgrd state, written into flares so a support
// engineer can see which agent processes the supervisor owns and, when one is not running, why.
//
// Failures are values in this struct rather than a returned error: "dd-procmgrd is not
// answering" is itself the answer, so it has to survive into the flare.
type SupportReport struct {
	CollectedAt time.Time `json:"collected_at"`
	// SocketPath is the IPC endpoint the Agent resolves for dd-procmgrd, honouring
	// DD_PM_SOCKET_PATH. Recorded because an unexpected value explains a failure to connect.
	SocketPath string         `json:"socket_path"`
	Daemon     DaemonSnapshot `json:"daemon"`
	// DaemonError is why dd-procmgrd could not be reached or could not answer, empty on success.
	DaemonError string `json:"daemon_error,omitempty"`
	// Processes is every process dd-procmgrd supervises, sorted by name, not just the ones in
	// the migratable service catalog.
	Processes []ProcessSnapshot `json:"processes"`
	// ProcessesError is why the supervised process list could not be retrieved.
	ProcessesError string `json:"processes_error,omitempty"`
	// Services maps each migratable agent service to the supervisor that actually owns it,
	// which is what tells a reader whether a Stopped legacy service is expected on this host.
	Services []ServiceSnapshot `json:"services"`
	// Warnings records per-process failures that did not stop the rest of the report.
	Warnings []string `json:"warnings,omitempty"`
	// Notes explains how to read a process that is not running.
	Notes []string `json:"notes"`
}

// reportNotes explains how to interpret a process that is not Running.
//
// dd-procmgrd does not report a start-block reason over its RPC: the wire format carries
// condition_path_exists but not condition_config_any, and there is no Blocked state, so a
// config-gated process is indistinguishable from one waiting on start ordering. The daemon logs
// the gate decision, and the flare already collects logs/dd-procmgr.log through the log
// directory sweep, so the notes point a reader there rather than guessing.
var reportNotes = []string{
	"state=running: the process is supervised and up.",
	"state=stopped: the process was stopped on request, e.g. an operator stop or an agent shutdown.",
	"state=crashed or state=failed with restart_count>0: a crash loop. See last_exit_code and last_signal.",
	"state=failed with restart_count=0: the spawn itself failed. See logs/dd-procmgr.log.",
	"state=created with auto_start=true: the process was never started, because a config gate " +
		"(condition_config_any) is closed, the condition_path_exists path is missing, or a start " +
		"ordering dependency is unmet. dd-procmgrd does not report which one over its RPC: search " +
		"logs/dd-procmgr.log for the gate decision.",
	"state=created with auto_start=false: an inert catalog entry, expected until the matching service is migrated.",
	"A legacy service reported Stopped in servicestatus.json is the expected state when the same " +
		"workload appears in the services list with management_mode=procmgr.",
}

// Report returns a dump of dd-procmgrd state for a flare. It never fails: every error is
// recorded in the returned report.
func (c *Collector) Report(ctx context.Context) SupportReport {
	ctx, cancel := clientContext(ctx)
	defer cancel()

	report := SupportReport{
		CollectedAt: time.Now().UTC(),
		SocketPath:  procmgrSocketPath(),
		Processes:   []ProcessSnapshot{},
		Services:    make([]ServiceSnapshot, 0, len(migratableServices)),
		Notes:       reportNotes,
	}

	processes := map[string]ProcessSnapshot{}

	sess, err := c.client.Connect(ctx)
	if err != nil {
		report.DaemonError = fmt.Sprintf("connect to dd-procmgrd: %v", err)
	} else {
		defer func() { _ = sess.Disconnect() }()

		daemon, err := sess.Status(ctx)
		if err != nil {
			report.DaemonError = fmt.Sprintf("dd-procmgrd status: %v", err)
		} else {
			report.Daemon = daemon
			processes, err = sess.List(ctx)
			if err != nil {
				processes = map[string]ProcessSnapshot{}
				report.ProcessesError = fmt.Sprintf("dd-procmgrd list: %v", err)
			} else {
				report.Processes, report.Warnings = describeAll(ctx, sess, processes)
			}
		}
	}

	for _, service := range migratableServices {
		report.Services = append(report.Services, c.collectService(ctx, service, processes))
	}

	report.Scrub(ScrubOptions{})

	return report
}

// ScrubOptions carries the operator's process-argument privacy settings. This package does not
// read the Agent config, so a caller that can has to pass them in: an operator who declared a word
// sensitive, or asked for arguments to be stripped outright, means it for a flare as well.
type ScrubOptions struct {
	// CustomSensitiveWords extends the default word list, from
	// process_config.custom_sensitive_words.
	CustomSensitiveWords []string
	// StripArguments drops every argument rather than redacting individual values, from
	// process_config.strip_proc_arguments.
	StripArguments bool
}

// Scrub redacts secrets the report carries. Report calls it with default options, but anything that
// writes a report to a file or sends it anywhere has to call it too, with the operator's settings:
// a report assembled any other way, in a test or by a future caller, has not been through it, and
// the cost of missing it is a leaked credential. It is safe to call more than once.
func (r *SupportReport) Scrub(opts ScrubOptions) {
	scrubProcessArgs(r.Processes, opts)
}

// argvSentinel stands in for the executable at the head of a command line. procutil's patterns
// require a space or a dash before a flag, which the first element of a joined command line does
// not have, so process-agent satisfies them by passing the executable as element 0. Passing a
// sentinel instead keeps a Command holding spaces out of the re-split below.
const argvSentinel = "dd-procmgr-argv"

// hyphenSpelledSecretWords covers the hyphenated spellings of words procutil's defaults only list
// with underscores, so "--api-key" is recognized as readily as "--api_key". They are expressed in
// procutil's own wildcard syntax, which keeps this a list of data rather than matching logic.
var hyphenSpelledSecretWords = []string{"*api*key*", "*auth*token*", "*access*token*"}

// scrubProcessArgs redacts secret values in the command lines of supervised processes.
//
// The scrubber a flare applies on the way out works line by line, and every element of an args
// array is serialized onto a line of its own. By the time it runs, "--password" and its value are
// no longer on the same line, so the pairing that identifies the value as a secret is gone. Here is
// the last point where it is still visible.
//
// procutil does the matching, which is the same implementation process-agent applies to the
// cmdlines it reports. That buys the wildcard custom words operators can configure, the
// platform-specific flags ("/p" and "/rp" on Windows), quoted values and case insensitivity,
// none of which a hand-written matcher here would keep up with.
func scrubProcessArgs(processes []ProcessSnapshot, opts ScrubOptions) {
	if opts.StripArguments {
		for i := range processes {
			processes[i].Args = nil
		}
		return
	}

	scrubber := procutil.NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(append(hyphenSpelledSecretWords, opts.CustomSensitiveWords...))

	for i := range processes {
		if len(processes[i].Args) == 0 {
			continue
		}

		cmdline := append([]string{argvSentinel}, processes[i].Args...)
		if scrubbed, changed := scrubber.ScrubCommand(cmdline); changed {
			// procutil re-splits on spaces once it has redacted something, so an argument that
			// held a space arrives as several. That only happens to a command line that carried a
			// secret, and the alternative is leaving the secret in place.
			processes[i].Args = scrubbed[1:]
		}
	}
}

// describeAll enriches each listed process with the fields only Describe carries, above all
// auto_start, without which a Created process cannot be told from an inert catalog entry. A
// process whose Describe fails keeps its List data and contributes a warning.
func describeAll(ctx context.Context, sess ProcmgrSession, processes map[string]ProcessSnapshot) ([]ProcessSnapshot, []string) {
	names := make([]string, 0, len(processes))
	for name := range processes {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]ProcessSnapshot, 0, len(names))
	var warnings []string
	for _, name := range names {
		detail, err := sess.Describe(ctx, name)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("dd-procmgrd describe %s: %v", name, err))
			out = append(out, processes[name])
			continue
		}
		out = append(out, detail)
	}
	return out, warnings
}
