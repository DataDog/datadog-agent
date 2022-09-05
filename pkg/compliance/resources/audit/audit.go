// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package audit

import (
	"errors"
	"os"

	"github.com/elastic/go-libaudit"
	"github.com/elastic/go-libaudit/rule"
	"github.com/elastic/go-libaudit/rule/flags"

	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewAuditClient returns a new audit client
func NewAuditClient() (env.AuditClient, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("you must be root to receive audit data")
	}

	client, err := libaudit.NewMulticastAuditClient(nil)
	if err != nil {
		return nil, err
	}

	return &auditClient{
		client: client,
	}, err
}

type auditClient struct {
	client *libaudit.AuditClient
}

func (c *auditClient) Close() error {
	return c.client.Close()
}

// GetFileWatchRules returns audit rules for file watching
func (c *auditClient) GetFileWatchRules() ([]*rule.FileWatchRule, error) {
	data, err := c.client.GetRules()
	if err != nil {
		return nil, err
	}

	var rules []*rule.FileWatchRule

	// Enumerate all rules and filter out file watch ones
	for _, d := range data {
		cmdline, err := rule.ToCommandLine(rule.WireFormat(d), false)
		if err != nil {
			log.Errorf("Failed to convert to command line: %v", err)
			continue
		}
		r, err := flags.Parse(cmdline)
		if err != nil {
			log.Errorf("Failed to parse rule: %s - %v", cmdline, err)
			continue
		}

		if r.TypeOf() != rule.FileWatchRuleType {
			log.Tracef("Skipped rule of type %d", r.TypeOf())
			continue
		}
		if r, ok := r.(*rule.FileWatchRule); ok {
			rules = append(rules, r)
		}
	}
	return rules, nil
}
