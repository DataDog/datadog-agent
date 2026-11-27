// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

// ASA is the Cisco ASA CLI.
var ASA = &Dialect{
	Name: "cisco-asa",
	// e.g. `fw01# `, `fw01>` or `fw01(config)# `.
	Prompt: regexp.MustCompile(`^[\w.@/-]+(\([\w.-]+\))?\s*[#>] ?$`),
	// The pager is on at the start of every session and would stop long
	// output at a "<--- More --->" marker.
	Setup: []string{"terminal pager 0"},
	Error: regexp.MustCompile(`(?m)^ERROR: .*$`),
	Exit:  "exit",
}

// IOS is the Cisco IOS CLI.
var IOS = &Dialect{
	Name: "cisco-ios",
	// e.g. `R1#`, `R1>` or `R1(config-if)#`.
	Prompt: regexp.MustCompile(`^[A-Za-z0-9._-]+(\([A-Za-z0-9._-]+\))?[>#] ?$`),
	// Don't paginate output, and don't wrap echoed commands.
	Setup: []string{"terminal length 0", "terminal width 511"},
	// Matches the line IOS prints when it rejects or fails to run a command.
	// Other lines starting with "%" are informational and are not matched,
	// e.g. "% Warning: ...", "% Note: ..." or syslog messages such as
	// "%SYS-5-CONFIG_I: ...".
	// Note that cisco does not publish a list of all possible errors and
	// warnings; this list should cover the vast majority of cases with very few
	// false positives, but it is difficult to be certain and new commands could
	// introduce new error messages in future versions of IOS.
	Error: regexp.MustCompile(`(?m)^(?:` + strings.Join([]string{
		// Parser rejections.
		`% Invalid `,           // % Invalid input detected at '^' marker. / % Invalid IP address / % Invalid interface type and number
		`% Incomplete command`, // % Incomplete command.
		`% Ambiguous command`,  // % Ambiguous command:  "sh i"
		`% Unrecognized command`,
		`% Unknown `, // % Unknown command or computer name, or unable to find computer address / % Unknown VRF specified
		`% Bad `,     // % Bad IP address or host name / % Bad mask
		// Generic failures, e.g. from copy, delete or file operations.
		`%Error`,  // %Error opening flash:foo (No such file or directory)
		`% Error`, // % Error in authentication.
		`% Cannot `,
		`% Can't `,
		`% Unable to `,
		`% Failed `,
		`% No such `,
		`% Not enough space`,
		`% Insufficient `,
		`% \S+ overlaps with `, // % 10.0.0.0 overlaps with GigabitEthernet0/1
		// Authorization failures.
		`% Command authorization failed`,
		`% Authorization failed`,
		`% Access denied`,
		`% Permission denied`,
	}, "|") + `).*$`),
	Exit: "exit",
}

var byProfile = map[profile.ProfileName]*Dialect{
	profile.ProfileCiscoASA: ASA,
	profile.ProfileCiscoIOS: IOS,
}

// ForProfile returns the CLI dialect for devices using the named profile, or
// an error if no interactive shell is defined for it.
func ForProfile(name profile.ProfileName) (*Dialect, error) {
	if d, ok := byProfile[name]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("no interactive shell defined for profile %q", name)
}
