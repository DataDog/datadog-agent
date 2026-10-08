// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package healthcheck

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// scrubCommand redacts secrets from a remediation command before it is written into an event.
func scrubCommand(command string) string {
	scrubbed, err := scrubber.ScrubString(command)
	if err != nil {
		return "[redacted]"
	}
	return strings.TrimSpace(scrubbed)
}

// remediationSource is the event source_type_name; "datadog" renders as the datadog source in the UI.
const remediationSource = "datadog"

// outcomeAlertType colors lifecycle events: detected=warning, remediated=success, escalate=error.
func outcomeAlertType(outcome string) event.AlertType {
	switch outcome {
	case "remediated":
		return event.AlertTypeSuccess
	case "escalate":
		return event.AlertTypeError
	default:
		return event.AlertTypeWarning
	}
}

// detectedText builds the "detected" event body: what went CRITICAL, why, and the steps to run.
func detectedText(scName, hostname, failureMessage string, id checkid.ID, cfg *integration.HealthCheckConfig) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Service check %q went CRITICAL on %s (check %s)", scName, hostname, id)
	if failureMessage != "" {
		fmt.Fprintf(&b, ": %s", failureMessage)
	}
	fmt.Fprintf(&b, ". Running %d remediation step(s):", len(cfg.Remediation.Steps))
	for i, step := range cfg.Remediation.Steps {
		fmt.Fprintf(&b, "\n%d. %s", i+1, scrubCommand(step.Command))
	}
	return b.String()
}

// EventDispatcher emits dry-run events without executing remediation commands.
type EventDispatcher struct {
	out      chan<- event.Event
	hostname string
}

// NewEventDispatcher uses the aggregator's event input for dry-run reports.
func NewEventDispatcher(out chan<- event.Event, hostname string) *EventDispatcher {
	return &EventDispatcher{out: out, hostname: hostname}
}

// Dispatch describes the steps that would run and never executes them.
func (d *EventDispatcher) Dispatch(ctx context.Context, id checkid.ID, scName, failureMessage string, cfg *integration.HealthCheckConfig) {
	if d == nil || d.out == nil || cfg == nil || ctx.Err() != nil {
		return
	}
	var text strings.Builder
	fmt.Fprintf(&text, "Would remediate check %s after service check %s became CRITICAL", id, scName)
	if failureMessage != "" {
		fmt.Fprintf(&text, ": %s", failureMessage)
	}
	text.WriteString(".\nSteps that would run:")
	for i, step := range cfg.Remediation.Steps {
		fmt.Fprintf(&text, "\n%d. %s", i+1, scrubCommand(step.Command))
	}
	e := event.Event{
		Title:          "health-check remediation (dry-run)",
		Text:           text.String(),
		Ts:             time.Now().Unix(),
		Host:           d.hostname,
		Priority:       event.PriorityNormal,
		AlertType:      event.AlertTypeWarning,
		SourceTypeName: remediationSource,
		AggregationKey: "health_check_remediation:" + string(id),
		Tags:           []string{"check_id:" + string(id), "service_check:" + scName, "remediation:dry-run"},
	}
	select {
	case d.out <- e:
		log.Infof("Health-check remediation (dry-run): would remediate check %s for service check %s (%d steps)", id, scName, len(cfg.Remediation.Steps))
	case <-ctx.Done():
	}
}
