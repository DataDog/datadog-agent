// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package darwin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DataDog/datadog-agent/pkg/security/darwin/eslogger"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
			raw, err := serializers.MarshalEvent(match.Event, c.scrubber)
			if err != nil {
				log.Warnf("serialize %s: %v", match.RuleID, err)
				continue
			}

			// The rule id has to travel as a TAG, not just in the RuleID field.
			// DirectEventMsgSender forwards msg.Tags to the log origin, and that is
			// what surfaces as @agent.rule_id in the backend -- which is in turn
			// what a Workload Protection detection rule matches on. Without this
			// the agent event arrives and renders, but no signal can ever be built
			// from it. pkg/security/module/server.go does the same thing on Linux.
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
