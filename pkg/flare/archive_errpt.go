// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"fmt"
	"time"
)

const (
	// errptMaxOutput caps each errpt section in the flare; hosts with a
	// flapping subsystem can generate a lot of unrelated SRC entries.
	errptMaxOutput = 256 * 1024
)

// errptSince formats t in the MMDDhhmmYY form expected by errpt's -s flag.
func errptSince(t time.Time) string {
	return t.Format("0102150406")
}

// buildErrptLog assembles errpt.log from the two errpt queries. Query errors
// are reported inline; the root hint is only written when both queries
// succeeded and returned nothing — a flare built by the agent process
// (running as dd-agent), which cannot read the error log.
func buildErrptLog(deaths []byte, errDeaths error, src []byte, errSrc error) []byte {
	content := bytes.NewBuffer(nil)

	content.WriteString("=== Process deaths by signal, core dumps and paging space events (errpt -a -j A924A5FC,620096CD,8527F6F4) ===\n")
	content.Write(capErrptOutput(deaths))
	if errDeaths != nil {
		content.WriteString(fmt.Sprintf("\n(errpt failed: %v)\n", errDeaths))
	}

	content.WriteString("\n\n=== SRC subsystem lifecycle (errpt -a -N SRC) ===\n")
	content.Write(capErrptOutput(src))
	if errSrc != nil {
		content.WriteString(fmt.Sprintf("\n(errpt failed: %v)\n", errSrc))
	}

	if errDeaths == nil && errSrc == nil && len(deaths) == 0 && len(src) == 0 {
		content.WriteString("\nerrpt returned no entries. The AIX error log is only readable by root; " +
			"to include it, run `sudo datadog-agent flare --local` so the CLI builds the flare as root " +
			"instead of the agent process.\n")
	}

	return content.Bytes()
}

// capErrptOutput truncates out at errptMaxOutput with a marker.
func capErrptOutput(out []byte) []byte {
	if len(out) <= errptMaxOutput {
		return out
	}
	capped := make([]byte, 0, errptMaxOutput+len(truncatedMarker)+1)
	capped = append(capped, out[:errptMaxOutput]...)
	capped = append(capped, truncatedMarker...)
	return capped
}

const truncatedMarker = "\n[output truncated]\n"
