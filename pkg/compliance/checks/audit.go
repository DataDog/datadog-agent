// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package checks

import (
	"os"

	"github.com/elastic/go-libaudit"
	"github.com/elastic/go-libaudit/rule"
	"github.com/elastic/go-libaudit/rule/flags"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func newAuditClient() AuditClient {
	if os.Geteuid() != 0 {
		log.Error("you must be root to receive audit data")
		return nil
	}

	client, err := libaudit.NewMulticastAuditClient(nil)
	if err != nil {
		log.Errorf("failed to initialize audit client: %v", err)
		return nil
	}

	return &auditClient{
		client: client,
	}
}

type auditClient struct {
	client *libaudit.AuditClient
}

func (c *auditClient) Close() error {
	return c.client.Close()
}

func (c *auditClient) GetFileWatchRules() ([]*rule.FileWatchRule, error) {
	data, err := c.client.GetRules()
	if err != nil {
		return nil, err
	}

	var rules []*rule.FileWatchRule
	for _, d := range data {
		cmdline, err := rule.ToCommandLine(rule.WireFormat(d), false)
		if err != nil {
			log.Errorf("failed to convert to command line: %s", err)
			continue
		}
		r, err := flags.Parse(cmdline)
		if err != nil {
			log.Errorf("failed to parse rule: %s", err)
			continue
		}

		if r.TypeOf() != rule.FileWatchRuleType {
			continue
		}
		if r, ok := r.(*rule.FileWatchRule); ok {
			rules = append(rules, r)
		}
	}
	return rules, nil
}
