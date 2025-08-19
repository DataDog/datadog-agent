// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package compliance

import (
	"fmt"
	"os"

	"github.com/elastic/go-libaudit/v2"
	"github.com/elastic/go-libaudit/v2/rule"
	"github.com/elastic/go-libaudit/v2/rule/flags"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FileWatchRule is used to audit access to particular files or directories
// that you may be interested in.
type FileWatchRule rule.FileWatchRule

// Resolve the file watch rule
func (r *FileWatchRule) Resolve() interface{} {
	permissions := ""
	for _, p := range r.Permissions {
		switch p {
		case rule.ReadAccessType:
			permissions += "r"
		case rule.WriteAccessType:
			permissions += "w"
		case rule.ExecuteAccessType:
			permissions += "e"
		case rule.AttributeChangeAccessType:
			permissions += "a"
		}
	}

	return map[string]interface{}{
		"path":        r.Path,
		"enabled":     true,
		"permissions": permissions,
	}
}

func newLinuxAuditClient() (LinuxAuditClient, error) {
	if os.Geteuid() != 0 {
		return nil, ErrIncompatibleEnvironment
	}
	client, err := libaudit.NewMulticastAuditClient(nil)
	if err != nil {
		return nil, err
	}
	return &linuxAuditClient{client}, nil
}

type linuxAuditClient struct {
	client *libaudit.AuditClient
}

func (c *linuxAuditClient) Close() error {
	return c.client.Close()
}

func (c *linuxAuditClient) GetFileWatchRules() ([]*FileWatchRule, error) {
	data, err := c.client.GetRules()
	if err != nil {
		return nil, fmt.Errorf("audit: failed to get rules: %w", err)
	}
	var rules []*FileWatchRule
	// Enumerate all rules and filter out file watch ones
	for _, d := range data {
		cmdline, err := rule.ToCommandLine(rule.WireFormat(d), false)
		if err != nil {
			log.Warnf("audit: failed to convert to command line: %v", err)
			continue
		}
		r, err := flags.Parse(cmdline)
		if err != nil {
			log.Warnf("audit: failed to parse rule: %s - %v", cmdline, err)
			continue
		}
		if r.TypeOf() != rule.FileWatchRuleType {
			log.Tracef("audit: skipped rule of type %d", r.TypeOf())
			continue
		}
		if r, ok := r.(*rule.FileWatchRule); ok {
			r2 := FileWatchRule(*r)
			rules = append(rules, &r2)
		}
	}
	return rules, nil
}
