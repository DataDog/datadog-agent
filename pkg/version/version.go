package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// AgentVersion holds SemVer infos for the agent
type AgentVersion struct {
	Major int64
	Minor int64
	Patch int64
	Pre   string
	Meta  string
}

// New parses a version string like `0.0.0` and returns an AgentVersion instance
func New(version string) (AgentVersion, error) {
	re := regexp.MustCompile(`(\d+\.\d+\.\d+)(\-[^\+]+)*(\+.+)*`)
	toks := re.FindStringSubmatch(version)
	if len(toks) == 0 || toks[0] != version {
		// if regex didn't match or partially matched, raise an error
		return AgentVersion{}, fmt.Errorf("Version string has wrong format")
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

	av := AgentVersion{
		Major: major,
		Minor: minor,
		Patch: patch,
		Pre:   pre,
		Meta:  meta,
	}

	return av, nil
}

func (v *AgentVersion) String() string {
	ver := v.GetNumber()
	if v.Pre != "" {
		ver = fmt.Sprintf("%s-%s", ver, v.Pre)
	}
	if v.Meta != "" {
		ver = fmt.Sprintf("%s+%s", ver, v.Meta)
	}

	return ver
}

// GetNumber returns a string containing version numbers only, e.g. `0.0.0`
func (v *AgentVersion) GetNumber() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}
