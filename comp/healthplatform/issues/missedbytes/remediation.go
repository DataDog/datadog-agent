// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package missedbytes

// What the issue tells a customer to do, kept apart from detection so a copy
// edit touches one file. Summary is plain text (not rendered as markdown).
const remediationSummary = "Give the Agent more time to finish reading rotated files, and relieve any saturation in the logs pipeline."

// Ordered cheapest to most invasive, per the component's remediation rules;
// steps 3-5 branch on the saturated component step 1 identifies.
var remediationSteps = []string{
	"Run `sudo datadog-agent status` and note any saturated component in the Logs Agent Backpressure section.",
	"Raise `logs_config.close_timeout` (DD_LOGS_CONFIG_CLOSE_TIMEOUT) above its 60 second default to give the tailer longer to finish a rotated file.",
	"If a `destination_reliable_N` or `worker` row is saturated, check the Agent log for failed or retried submissions and resolve any proxy, DNS, authentication, or connectivity errors.",
	"If the `strategy` row is saturated, set `logs_config.use_compression` to false or raise `logs_config.pipelines`.",
	"If the `processor` row is saturated, scope global `logs_config.processing_rules` to the affected source, or set `logs_config.auto_multi_line_detection` to false.",
	"If the Agent still cannot keep up, drop unneeded logs with an `exclude_at_match` processing rule, or reduce the volume written to the file between rotations.",
	"Re-run `sudo datadog-agent status` under representative log volume and confirm no new rotation warnings appear in the Agent log. This issue clears about an hour after the last loss, or within 30 minutes of running `sudo datadog-agent logs clear-health` once the cause is fixed.",
}
