// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsdropped

import (
	"fmt"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

// What the issue tells a customer to do, kept apart from detection so a copy edit
// touches one file. Summary is plain text (not rendered as markdown).
const remediationSummary = "Fix what makes the intake or a destination reject logs, so the Agent stops dropping logs it has already read."

const (
	firstStep = "Run `sudo datadog-agent status` and check the Logs Agent section for the endpoints in use, their API key status, and any errors."
	logStep   = "Search the Agent log for rejected or failed sends to see each status code and endpoint: `grep -iE \"failed to post http payload|could not send payload|dropped message|dropping payload\" /var/log/datadog/agent.log` (on Windows, `C:\\ProgramData\\Datadog\\logs\\agent.log`). An endpoint whose buffer fills logs nothing."
	lastStep  = "Run `sudo datadog-agent status` again and confirm the Logs Agent section shows no new errors. This issue clears about an hour after the last dropped log, or within 30 minutes of running `sudo datadog-agent logs clear-health` once the cause is fixed."
)

// remediation lists the shared diagnosis, then one step per reason present, in
// reasonOrder, naming the endpoints it was seen at.
func remediation(f facts) *healthplatform.Remediation {
	texts := []string{firstStep, logStep}
	for _, r := range f.Reasons {
		texts = append(texts, reasonStep(r, f.Endpoints[r]))
	}
	texts = append(texts, lastStep)
	return &healthplatform.Remediation{Summary: remediationSummary, Steps: logsissue.Steps(texts...)}
}

func reasonStep(r logsmetrics.DropReason, hosts []string) string {
	at := ""
	if len(hosts) > 0 {
		at = " (" + strings.Join(hosts, ", ") + ")"
	}
	switch r {
	case logsmetrics.DropReasonAuth:
		return fmt.Sprintf("The intake%s rejected the API key, or the Agent had none to send. Check that `api_key`, or the `api_key` of the matching entry under `logs_config.additional_endpoints`, is a valid key for that endpoint's Datadog site. If it comes from a secrets backend, run `sudo datadog-agent secret` to check that it resolves. Then restart the Agent.", at)
	case logsmetrics.DropReasonTooLarge:
		return fmt.Sprintf("Payloads were too large for the intake%s, or single log lines were larger than a whole batch. If `logs_config.batch_max_content_size` or `logs_config.batch_max_size` was raised, set it back to its default (5000000 bytes and 1000 logs), and keep `logs_config.max_message_size_bytes` (default 900000) below `logs_config.batch_max_content_size` so long lines are truncated instead of dropped.", at)
	case logsmetrics.DropReasonBufferFull:
		return fmt.Sprintf("The endpoint%s fell behind until its buffer filled. Check connectivity from this host to it (proxy, DNS, firewall) and the Agent log for its send errors.", at)
	case logsmetrics.DropReasonSendFailed:
		return fmt.Sprintf("Sends to a best-effort endpoint%s failed. Endpoints with `is_reliable: false` never retry, so any network error loses that copy: check connectivity to it, or set `is_reliable: true` if every log must arrive there.", at)
	case logsmetrics.DropReasonMalformed:
		return fmt.Sprintf("The intake%s rejected payloads as malformed, or the Agent could not encode them. Run `sudo datadog-agent flare` and send it to Datadog support.", at)
	}
	return "Run `sudo datadog-agent flare` and send it to Datadog support."
}
