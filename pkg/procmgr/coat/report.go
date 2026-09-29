// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"context"
	"fmt"
	"slices"
	"strings"
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
//
// opts carries the operator's process-argument privacy settings, and has to arrive here rather than
// be applied to the result later. Redacting a value overwrites the argument that held it, so a pass
// with a narrower word list destroys the flag names a later pass would need: given
// ["--password", "--tenant-thing", "s3cret"], a default-words pass leaves the middle argument a
// placeholder, and "*tenant*" can no longer be recognized there.
func (c *Collector) Report(ctx context.Context, opts ScrubOptions) SupportReport {
	ctx, cancel := clientContext(ctx)
	defer cancel()

	out := SupportReport{
		CollectedAt: time.Now().UTC(),
		SocketPath:  procmgrSocketPath(),
		Processes:   []ProcessSnapshot{},
		Services:    make([]ServiceSnapshot, 0, len(migratableServices)),
		Notes:       reportNotes,
	}

	processes := map[string]ProcessSnapshot{}

	sess, err := c.client.Connect(ctx)
	if err != nil {
		out.DaemonError = fmt.Sprintf("connect to dd-procmgrd: %v", err)
	} else {
		defer func() { _ = sess.Disconnect() }()

		daemon, err := sess.Status(ctx)
		if err != nil {
			out.DaemonError = fmt.Sprintf("dd-procmgrd status: %v", err)
		} else {
			out.Daemon = daemon
			processes, err = sess.List(ctx)
			if err != nil {
				processes = map[string]ProcessSnapshot{}
				out.ProcessesError = fmt.Sprintf("dd-procmgrd list: %v", err)
			} else {
				out.Processes, out.Warnings = describeAll(ctx, sess, processes)
			}
		}
	}

	for _, service := range migratableServices {
		out.Services = append(out.Services, c.collectService(ctx, service, processes))
	}

	out.Scrub(opts)

	return out
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

// redactedValue replaces a secret. It matches the placeholder procutil substitutes, so redactions
// look the same wherever support reads them.
const redactedValue = "********"

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
// procutil decides which flags name a secret, which is the same judgement process-agent applies to
// the cmdlines it reports: wildcard custom words, the platform-specific flags ("/p" and "/rp" on
// Windows) and case insensitivity all come from there rather than from anything written here.
func scrubProcessArgs(processes []ProcessSnapshot, opts ScrubOptions) {
	if opts.StripArguments {
		for i := range processes {
			processes[i].Args = nil
		}
		return
	}

	scrubber := procutil.NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(slices.Concat(hyphenSpelledSecretWords, opts.CustomSensitiveWords))

	for i := range processes {
		redactSecretValues(processes[i].Args, scrubber.SensitivePatterns)
	}
}

// redactSecretValues rewrites, in place, the value of every argument whose flag names a secret.
//
// It works per argv element rather than handing the command line to procutil.ScrubCommand: that call
// joins the argv on spaces, and its unquoted-value pattern stops at the first space, so a value like
// "secret with spaces" would keep everything after "secret". Element boundaries are known here, and
// throwing them away loses information that cannot be recovered.
func redactSecretValues(args []string, patterns []procutil.DataScrubberPattern) {
	// Classified up front, because redacting rewrites the element after a flag. Classifying as the
	// rewriting goes along would read a placeholder where a flag used to be: in
	// ["--password", "--api-key", "s3cret"], "--api-key" would be overwritten before it was ever
	// recognized and "s3cret" would reach the flare.
	classified := classifyArguments(args, patterns)
	// Elements replaced wholesale because they were a secret's value. They are not examined again:
	// their classification describes text that is gone, and reapplying it would rebuild part of it.
	wasValue := make([]bool, len(args))

	for i, arg := range classified {
		if !arg.namesSecret {
			continue
		}
		if arg.hasInlineValue {
			// Unless this element is itself a redacted value now, in which case its classification
			// describes text that is gone and rebuilding from it would restore part of the secret.
			if !wasValue[i] {
				args[i] = arg.flag + arg.delimiter + redactedValue
			}
			continue
		}
		// The value is the following element, as in ["--password", "s3cret"], and all of it goes
		// whatever it looks like: a value is not disqualified from being one by starting with a
		// dash or a slash, and "--password /etc/creds" is an ordinary way to write one. A flag
		// that took no value is worth losing to a redaction, a credential is not worth risking on
		// a guess. Nor does the name have to be spelled as a flag, because procutil recognizes a
		// bare "password" and a flare has no business keeping what procutil would redact.
		//
		// The exception is an argument that has itself been replaced as a value, which does not go
		// on to claim the argument behind it. Otherwise a value that happens to name a secret,
		// "my-api-key-value" say, would swallow the unrelated argument that follows. One written
		// as a flag is not treated that way: in ["--password", "--api-key", "s3cret"] the middle
		// argument is a flag with a value of its own, and that value still has to go.
		if (!wasValue[i] || arg.isFlag) && i+1 < len(args) {
			args[i+1] = redactedValue
			wasValue[i+1] = true
		}
	}
}

// argument is one argv element, split and classified before any redaction rewrites it.
type argument struct {
	flag           string
	delimiter      string
	hasInlineValue bool
	namesSecret    bool
	isFlag         bool
}

func classifyArguments(args []string, patterns []procutil.DataScrubberPattern) []argument {
	classified := make([]argument, len(args))
	for i, arg := range args {
		flag, delimiter, hasInlineValue := splitArgument(arg)
		classified[i] = argument{
			flag:           flag,
			delimiter:      delimiter,
			hasInlineValue: hasInlineValue,
			namesSecret:    namesSecret(patterns, flag),
			isFlag:         looksLikeFlag(arg),
		}
	}
	return classified
}

// looksLikeFlag reports whether an argument is written the way a flag is written, which is what
// decides whether it can carry its value in the argument after it.
//
// It says nothing about whether an argument names a secret, and nothing about whether one is a
// value: "password=x" carries a secret without being spelled as a flag, and is redacted on its own
// token, while "/etc/creds" is a perfectly ordinary value that happens to start with a slash.
//
// Both prefixes count, because procutil recognizes the Windows "/p" and "/rp" spellings alongside
// dashed ones.
func looksLikeFlag(arg string) bool {
	return strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "/")
}

// argumentDelimiters are the characters that can separate a flag from its value inside a single
// argument token. Missing one is worse than useless: the value stays intact and the unrelated
// argument that follows gets redacted in its place. Whitespace is included because procmgr reads
// args from a YAML list, where writing a flag and its value as one entry is an easy thing to do.
const argumentDelimiters = "=: \t"

// splitArgument separates the flag in an argument from a value carried in the same token.
//
// A token with no flag, such as a bare "C:\Program Files\Datadog\datadog.yaml", splits at its
// first delimiter into a harmless "C" that names no secret, so paths pass through untouched.
func splitArgument(arg string) (flag, delimiter string, hasInlineValue bool) {
	i := strings.IndexAny(arg, argumentDelimiters)
	if i < 0 {
		return arg, "", false
	}
	return arg[:i], arg[i : i+1], true
}

// namesSecret reports whether a flag names a secret, according to procutil's compiled patterns.
//
// Those patterns describe a whole "flag delimiter value" sequence, so the flag is probed inside the
// smallest one that can match: a leading space, the flag, and a stand-in value. The leading space
// matters because the patterns require a space or a dash ahead of the flag.
func namesSecret(patterns []procutil.DataScrubberPattern, flag string) bool {
	probe := " " + flag + "=x"
	for _, pattern := range patterns {
		if pattern.Re.MatchString(probe) {
			return true
		}
	}
	return false
}

// describeAll enriches each listed process with the fields only Describe carries, above all
// auto_start, without which a Created process cannot be told from an inert catalog entry. A
// process whose Describe fails keeps its List data and contributes a warning.
func describeAll(ctx context.Context, sess ProcmgrSession, processes map[string]ProcessSnapshot) ([]ProcessSnapshot, []string) {
	names := make([]string, 0, len(processes))
	for name := range processes {
		names = append(names, name)
	}
	slices.Sort(names)

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
