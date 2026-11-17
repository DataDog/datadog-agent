// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package untailedfiles

import (
	"fmt"
	"strconv"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
)

// What the issue tells a customer to do, kept apart from detection so a copy edit
// touches one file. Summary is plain text (not rendered as markdown).
const remediationSummary = "Let the Agent tail every file your log configurations match, by raising the open files limit or matching fewer files."

// remediation orders the steps cheapest to most invasive, per the component's
// remediation rules, with the real limit and file count filled in.
func remediation(limit, matched int64) *healthplatform.Remediation {
	raiseTo := "the number of files you need to tail"
	if matched > 0 {
		raiseTo = "at least " + strconv.FormatInt(matched, 10)
	}
	return &healthplatform.Remediation{
		Summary: remediationSummary,
		Steps: logsissue.Steps(
			"Run `sudo datadog-agent status`. In the Logs Agent section, each wildcard source shows how many of its matching files are tailed, and a warning says the open files limit was reached.",
			fmt.Sprintf("Raise `logs_config.open_files_limit` (DD_LOGS_CONFIG_OPEN_FILES_LIMIT) from %d to %s. On Linux and macOS, also keep the Agent process's file descriptor limit (`ulimit -n`, or `LimitNOFILE` for its service) above that value.", limit, raiseTo),
			"Narrow wildcard `path` patterns to the files you need, or skip rotated and archived files with `exclude_paths` (for example `/var/log/app/*.gz`).",
			"If the limit cannot go higher, set `logs_config.file_wildcard_selection_mode` to `by_modification_time` so the most recently written files are tailed first.",
			"Run `sudo datadog-agent status` again and confirm every matching file is tailed. This issue clears an hour after the limit was last reached.",
		),
	}
}
