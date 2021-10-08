// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version holds SemVer infos for the agent and friends
type Version struct {
	Major  int64
	Minor  int64
	Patch  int64
	Pre    string
	Meta   string
	Commit string
}

var versionRx = regexp.MustCompile(`(\d+\.\d+\.\d+)(\-[^\+]+)*(\+.+)*`)

// Agent returns the Datadog Agent version.
func Agent() (Version, error) {
	return New(AgentVersion, Commit)
}

// New parses a version string like `0.0.0` and a commit identifier and returns a Version instance
func New(version, commit string) (Version, error) {
	toks := versionRx.FindStringSubmatch(version)
	if len(toks) == 0 || toks[0] != version {
		// if regex didn't match or partially matched, raise an error
		return Version{}, fmt.Errorf("Version string has wrong format")
	}

	// split version info (group 1 in regexp)
	parts := strings.Split(toks[1], ".")
	major, _ := strconv.ParseInt(parts[0], 10, 64)
	minor, _ := strconv.ParseInt(parts[1], 10, 64)
	patch, _ := strconv.ParseInt(parts[2], 10, 64)

	// save Pre infos after removing leading `-`
	pre := strings.Replace(toks[2], "-", "", 1)

	// save Meta infos after removing leading `+`
	meta := strings.Replace(toks[3], "+", "", 1)

	av := Version{
		Major:  major,
		Minor:  minor,
		Patch:  patch,
		Pre:    pre,
		Meta:   meta,
		Commit: commit,
	}

	return av, nil
}

func (v *Version) String() string {
	ver := v.GetNumber()
	if v.Pre != "" {
		ver = fmt.Sprintf("%s-%s", ver, v.Pre)
	}
	if v.Meta != "" {
		ver = fmt.Sprintf("%s+%s", ver, v.Meta)
	}
	if v.Commit != "" {
		if v.Meta != "" {
			ver = fmt.Sprintf("%s.commit.%s", ver, v.Commit)
		} else {
			ver = fmt.Sprintf("%s+commit.%s", ver, v.Commit)
		}
	}

	return ver
}

// GetNumber returns a string containing version numbers only, e.g. `0.0.0`
func (v *Version) GetNumber() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// GetNumberAndPre returns a string containing version number and the pre only, e.g. `0.0.0-beta.1`
func (v *Version) GetNumberAndPre() string {
	version := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		version = fmt.Sprintf("%s-%s", version, v.Pre)
	}
	return version
}
