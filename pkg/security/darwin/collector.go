// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package darwin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DataDog/datadog-agent/pkg/security/darwin/eslogger"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// EventSender ships a security event to the intake.
type EventSender interface {
	Send(msg *api.SecurityEventMessage, expire func(*api.SecurityEventMessage))
}

// CollectorConfig configures the macOS collector.
type CollectorConfig struct {
	Events      []string
	PoliciesDir string
	Hostname    string
	Sender      EventSender
}

// ruleEvaluator is the subset of *rules.RuleSet the collector uses.
type ruleEvaluator interface {
	Evaluate(event eval.Event) bool
}

// Collector wires eslogger to the SECL rule engine and the intake.
type Collector struct {
	cfg        CollectorConfig
	translator *Translator
	recorder   *MatchRecorder
	scrubber   *utils.Scrubber
	ruleSet    ruleEvaluator

	sent uint64
}

// NewCollector builds a collector.
func NewCollector(cfg CollectorConfig) (*Collector, error) {
	if cfg.Sender == nil {
		return nil, errors.New("sender is required")
	}
	if cfg.Hostname == "" {
		return nil, errors.New("hostname is required: without it the intake cannot attribute events")
	}

	// nil/nil still installs procutil's default sensitive-word set plus the
	// *token* / *jwt* additions, which is what argv redaction relies on.
	scrubber, err := utils.NewScrubber(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("scrubber: %w", err)
	}

	pr, err := process.NewEBPFLessResolver(nil, nil, scrubber, process.NewResolverOpts())
	if err != nil {
		return nil, fmt.Errorf("process resolver: %w", err)
	}

	fh := NewFieldHandlers(pr, cfg.Hostname)
	// NewTranslator builds its own user/group resolver via the platform seam in
	// names_darwin.go, so the collector does not need to know about usergroup.
	translator := NewTranslator(pr, fh)
	recorder := &MatchRecorder{}

	rs, err := NewRuleSet(cfg.PoliciesDir, func() eval.Event { return translator.newEvent() })
	if err != nil {
		return nil, err
	}
	rs.AddListener(recorder)

	log.Infof("macOS CWS collector loaded %d rules from %s", len(rs.GetRules()), cfg.PoliciesDir)

	return &Collector{
		cfg:        cfg,
		translator: translator,
		recorder:   recorder,
		scrubber:   scrubber,
		ruleSet:    rs,
	}, nil
}

// Run consumes eslogger until the context is cancelled or the stream ends.
func (c *Collector) Run(ctx context.Context) error {
	// Snapshot before subscribing. Everything already running -- the login shell,
	// the terminal, the package manager that started the activity -- is invisible
	// to the event stream, so without this every process tree truncates at
	// startup. A failure degrades the trees but must not stop collection.
	//
	// This leaves a small race: a process created between the snapshot and the
	// subscription is in neither. The orphan-exec counter in the shutdown summary
	// measures how often that actually happens.
	if n, err := Snapshot(c.translator.resolver, c.translator.userGroup); err != nil {
		log.Warnf("process snapshot failed, trees will be truncated: %v", err)
	} else {
		log.Infof("snapshotted %d running processes", n)
	}

	runner := eslogger.NewRunner(c.cfg.Events)

	stdout, err := runner.Start(ctx)
	if err != nil {
		return err
	}

	log.Infof("subscribed to Endpoint Security events: %v", c.cfg.Events)

	decoder := eslogger.NewDecoder(stdout)

	for {
		msg, err := decoder.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("decode: %w", err)
		}

		event, err := c.translator.Translate(msg)
		if err != nil {
			log.Debugf("translate: %v", err)
			continue
		}
		if event == nil {
			continue
		}

		c.traceEvent(event)

		c.recorder.Reset()
		c.ruleSet.Evaluate(event)

		for _, match := range c.recorder.Matches() {
			raw, err := c.buildPayload(match)
			if err != nil {
				log.Warnf("serialize %s: %v", match.RuleID, err)
				continue
			}

			// rule_id also travels as a tag, which is how it becomes a facet.
			// The attribute that detection rules match on is @agent.rule_id, and
			// that comes from the payload body -- see buildPayload.
			tags := []string{"rule_id:" + match.RuleID}
			tags = append(tags, match.Event.GetTags()...)

			c.cfg.Sender.Send(&api.SecurityEventMessage{
				RuleID:    match.RuleID,
				Data:      raw,
				Tags:      tags,
				Service:   "runtime-security-agent",
				Hostname:  c.cfg.Hostname,
				Timestamp: timestamppb.New(time.Now()),
			}, func(*api.SecurityEventMessage) {})

			c.sent++
			log.Infof("sent signal for rule %s (pid %d)", match.RuleID, match.Event.PIDContext.Pid)
		}
	}

	c.logSummary(decoder.Stats(), runner.Stderr())

	// Cancelling the context is how the collector shuts down, and it kills
	// eslogger, so Wait reports "signal: killed". That is a clean stop, not a
	// failure, and reporting it as one made the driver exit non-zero on a normal
	// Ctrl-C.
	err = runner.Wait()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// buildPayload produces the wire payload for a rule match.
//
// The serialized event is only half of it. A Workload Protection detection rule
// matches on @agent.rule_id, which is an ATTRIBUTE of the payload body, not a
// log tag -- so the event has to be wrapped in an events.BackendEvent supplying
// the "agent" object and a "title". Sending the rule id only as a tag makes it a
// facet, which looks right in the events explorer but cannot be turned into a
// signal.
//
// pkg/security/module/server.go does the same on Linux and merges the two JSON
// objects with an unexported mergeJSON; the same concatenation is repeated here
// rather than exporting it.
func (c *Collector) buildPayload(match Match) ([]byte, error) {
	eventJSON, err := serializers.MarshalEvent(match.Event, c.scrubber)
	if err != nil {
		return nil, fmt.Errorf("serialize event: %w", err)
	}

	backendEvent := events.BackendEvent{
		AgentContext: events.AgentContext{
			RuleID:         match.RuleID,
			OriginalRuleID: match.RuleID,
			Version:        version.AgentVersion,
			OS:             runtime.GOOS,
			Arch:           utils.RuntimeArch(),
			// Origin distinguishes this event source from the eBPF and ptrace
			// ones, which matters as soon as macOS events sit next to Linux ones.
			Origin: "eslogger",
		},
	}
	if match.Rule != nil && match.Rule.PolicyRule != nil {
		if def := match.Rule.Def; def != nil {
			backendEvent.Title = def.Description
			backendEvent.AgentContext.RuleVersion = def.Version
		}
		backendEvent.AgentContext.PolicyName = match.Rule.Policy.Name
		backendEvent.AgentContext.PolicyVersion = match.Rule.Policy.Version
	}

	backendJSON, err := json.Marshal(backendEvent)
	if err != nil {
		return nil, fmt.Errorf("serialize agent context: %w", err)
	}

	return mergeJSONObjects(backendJSON, eventJSON)
}

// mergeJSONObjects concatenates two JSON objects into one.
func mergeJSONObjects(j1, j2 []byte) ([]byte, error) {
	if len(j1) < 2 || len(j2) < 2 {
		return nil, errors.New("malformed json")
	}
	merged := append(j1[:len(j1)-1], ',')
	return append(merged, j2[1:]...), nil
}

// traceEvent logs the exact values the rule engine will match on: the event's
// own file name and its ancestor lineage. Without this, a rule that does not
// fire is indistinguishable from an event that never arrived, and the difference
// matters -- for interpreted entry points Endpoint Security reports the
// interpreter as the executable, not the script.
//
// Guarded on the log level because building the lineage string allocates per
// event, and exec volume on a laptop is high.
func (c *Collector) traceEvent(event *model.Event) {
	if !log.ShouldLog(log.TraceLvl) {
		return
	}

	eventType := event.GetEventType()
	if eventType != model.ExecEventType && eventType != model.ForkEventType {
		return
	}

	var lineage []string
	for entry := event.ProcessCacheEntry; entry != nil; entry = entry.Ancestor {
		name := entry.Process.FileEvent.BasenameStr
		if name == "" {
			name = "<none>"
		}
		lineage = append(lineage, fmt.Sprintf("%s(%d)", name, entry.Process.Pid))
		if len(lineage) > 12 {
			lineage = append(lineage, "...")
			break
		}
	}

	// argv matters here: when the executable is an interpreter, the script path
	// appears in the arguments rather than in file.path.
	argv := event.FieldHandlers.ResolveProcessArgvScrubbed(event, &event.ProcessContext.Process)

	log.Tracef("%s pid=%d file.name=%q file.path=%q argv0=%q argv=%v lineage=%s",
		eventType,
		event.PIDContext.Pid,
		event.ProcessContext.Process.FileEvent.BasenameStr,
		event.ProcessContext.Process.FileEvent.PathnameStr,
		event.ProcessContext.Process.Argv0,
		argv,
		strings.Join(lineage, " <- "),
	)
}

// logSummary reports what the run saw. The counters matter as much as the events:
// a run that decodes nothing usually means Full Disk Access was never granted,
// and dropped messages mean the process tree is incomplete.
func (c *Collector) logSummary(stats eslogger.Stats, stderr []string) {
	log.Infof("eslogger stream ended: %d lines, %d decoded, %d unknown, %d malformed, %d dropped",
		stats.Lines, stats.Decoded, stats.Unknown, stats.Malformed, stats.Dropped)
	log.Infof("collector: %d signals sent, %d recycled pids, %d orphan execs",
		c.sent, c.translator.RecycledPIDs, c.translator.OrphanExecs)

	if stats.Dropped > 0 {
		log.Warnf("Endpoint Security dropped %d messages; process trees may be incomplete", stats.Dropped)
	}
	if stats.Lines == 0 {
		log.Warn("eslogger produced no output at all. The usual cause is missing Full Disk Access " +
			"for the hosting terminal, which lets eslogger start but emit nothing.")
	}
	for _, line := range stderr {
		log.Warnf("eslogger: %s", line)
	}
}
