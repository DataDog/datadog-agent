// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

// SECLRuleOpts defines SECL rules options
type SECLRuleOpts struct {
	EnableKill bool
	AllowList  bool
	Lineage    bool
	ImageName  string
	ImageTag   string
	Service    string
	FIM        bool
}

// SeccompProfile represents a Seccomp profile
type SeccompProfile struct {
	DefaultAction string          `yaml:"defaultAction" json:"defaultAction"`
	Syscalls      []SyscallPolicy `yaml:"syscalls" json:"syscalls"`
}

// SyscallPolicy represents the policy in a seccomp profile
type SyscallPolicy struct {
	Names  []string `yaml:"names" json:"names"`
	Action string   `yaml:"action" json:"action"`
}

// GenerateRules return rules from activity dumps
func GenerateRules(ads []*profile.Profile, opts SECLRuleOpts) []*rules.RuleDefinition {
	var ruleDefs []*rules.RuleDefinition
	groupID := getGroupID(opts)

	lineage := make(map[string][]string)
	fims := make(map[string][]string)

	for _, ad := range ads {
		fimPathsperExecPath, execAndParent := ad.ActivityTree.ExtractPaths(opts.AllowList, opts.FIM, opts.Lineage)

		for execPath, fimPaths := range fimPathsperExecPath {
			tmp, ok := fims[execPath]
			if ok {
				fims[execPath] = append(tmp, fimPaths...)
			} else {
				fims[execPath] = fimPaths
			}
		}

		for p, pp := range execAndParent {
			tmp, ok := lineage[p]
			if ok {
				lineage[p] = append(tmp, pp...)
			} else {
				lineage[p] = pp
			}
		}
	}

	// add exec rules
	if opts.AllowList {
		var execs []string
		for e := range fims {
			execs = append(execs, e)
		}
		ruleDefs = append(ruleDefs, addRule(fmt.Sprintf(`exec.file.path not in [%s]`, strings.Join(execs, ", ")), groupID, opts))
	}

	// add fim rules
	if opts.FIM {
		for exec, paths := range fims {
			if len(paths) != 0 {
				ruleDefs = append(ruleDefs, addRule(fmt.Sprintf(`open.file.path not in [%s] && process.file.path == %s`, strings.Join(paths, ", "), exec), groupID, opts))
			}
		}
	}
	// add lineage
	if opts.Lineage {
		var (
			parentOp = "=="
			ctxOp    = "!="
		)
		var expressions []string
		for p, pp := range lineage {
			for _, ppp := range pp {
				if ppp == "" {
					parentOp = "!="
					ctxOp = "=="
				}
				expressions = append(expressions, fmt.Sprintf(`exec.file.path == "%s" && process.parent.file.path %s "%s" && process.parent.container.id %s ""`, p, parentOp, ppp, ctxOp))
			}
		}

		ruleDefs = append(ruleDefs, addRule(fmt.Sprintf(`!(%s)`, strings.Join(expressions, " || ")), groupID, opts))

	}
	return ruleDefs
}

// GenerateSeccompProfile returns a seccomp a profile
func GenerateSeccompProfile(ads []*profile.Profile) *SeccompProfile {

	sp := &SeccompProfile{
		DefaultAction: "SCMP_ACT_KILL",
		Syscalls: []SyscallPolicy{
			{
				Action: "SCMP_ACT_ALLOW",
				Names:  []string{},
			},
		},
	}

	for _, ad := range ads {
		syscalls := ad.ActivityTree.ExtractSyscalls(ad.Metadata.Arch)
		sp.Syscalls[0].Names = append(sp.Syscalls[0].Names, syscalls...)

	}
	slices.Sort(sp.Syscalls[0].Names)
	sp.Syscalls[0].Names = slices.Compact(sp.Syscalls[0].Names)
	return sp
}
func addRule(expression string, groupID string, opts SECLRuleOpts) *rules.RuleDefinition {
	ruleDef := &rules.RuleDefinition{
		Expression: expression,
		GroupID:    groupID,
		ID:         strings.Replace(uuid.New().String(), "-", "_", -1),
	}
	applyContext(ruleDef, opts)
	if opts.EnableKill {
		applyKillAction(ruleDef)
	}
	return ruleDef
}

func applyContext(ruleDef *rules.RuleDefinition, opts SECLRuleOpts) {
	var context []string

	if opts.ImageName != "" {
		context = append(context, fmt.Sprintf(`"short_image:%s"`, opts.ImageName))
	}
	if opts.ImageTag != "" {
		context = append(context, fmt.Sprintf(`"image_tag:%s"`, opts.ImageTag))
	}
	if opts.Service != "" {
		context = append(context, fmt.Sprintf(`"service:%s"`, opts.Service))
	}

	if len(context) == 0 {
		return
	}

	ruleDef.Expression = fmt.Sprintf("%s && (%s)", ruleDef.Expression, fmt.Sprintf(`container.tags in [%s]`, strings.Join(context, ", ")))
}

func applyKillAction(ruleDef *rules.RuleDefinition) {
	ruleDef.Actions = []*rules.ActionDefinition{
		{
			Kill: &rules.KillDefinition{
				Signal: "SIGKILL",
			},
		},
	}
}
func getGroupID(opts SECLRuleOpts) string {
	groupID := "rules_"
	if len(opts.ImageName) != 0 {
		groupID = fmt.Sprintf("%s%s", groupID, opts.ImageName)
	} else {
		groupID = fmt.Sprintf("%s%s", groupID, strings.Replace(uuid.New().String(), "-", "_", -1)) // It should be unique so that we can target it at least, but ImageName should be always set
	}
	if len(opts.ImageTag) != 0 {
		groupID = fmt.Sprintf("%s_%s", groupID, opts.ImageTag)
	}

	return groupID
}

// LoadActivityDumpsFromFiles load ads from a file or a directory
func LoadActivityDumpsFromFiles(path string) ([]*profile.Profile, error) {
	fileInfo, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("the path %s does not exist", path)
	} else if err != nil {
		return nil, fmt.Errorf("error checking the path: %s", err)
	}

	if fileInfo.IsDir() {
		dir, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open directory: %s", err)
		}
		defer dir.Close()

		// Read the directory contents
		files, err := dir.Readdir(-1)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %s", err)
		}

		profiles := []*profile.Profile{}
		for _, file := range files {
			p := profile.New()
			err := p.Decode(filepath.Join(path, file.Name()))
			if err != nil {
				return nil, fmt.Errorf("couldn't decode secdump: %w", err)
			}
			profiles = append(profiles, p)
		}
		return profiles, nil

	}
	// It's a file otherwise
	p := profile.New()
	err = p.Decode(path)
	if err != nil {
		return nil, fmt.Errorf("couldn't decode secdump: %w", err)
	}
	return []*profile.Profile{p}, nil
}
