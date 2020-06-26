// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/elastic/go-libaudit/rule"
)

// AuditClient defines the interface for interacting with the auditd client
type AuditClient interface {
	GetFileWatchRules() ([]*rule.FileWatchRule, error)
	Close() error
}

type auditCheck struct {
	baseCheck

	client AuditClient

	audit *compliance.Audit
}

func newAuditCheck(baseCheck baseCheck, client AuditClient, audit *compliance.Audit) (*auditCheck, error) {

	if len(audit.Path) == 0 {
		return nil, errors.New("unable to create audit check without a path")
	}
	return &auditCheck{
		baseCheck: baseCheck,
		client:    client,
		audit:     audit,
	}, nil
}

func (c *auditCheck) Run() error {
	log.Debugf("%s: running audit check - path %s", c.id, c.audit.Path)

	rules, err := c.client.GetFileWatchRules()
	if err != nil {
		return err
	}

	// Scal for the rule matching configured path
	for _, r := range rules {
		if r.Path == c.audit.Path {
			log.Debugf("%s: audit check - match %s", c.id, c.audit.Path)
			return c.reportOnRule(r)
		}
	}

	// If no rule found we still report this as "not enabled"
	return c.reportOnRule(nil)
}

func (c *auditCheck) reportOnRule(r *rule.FileWatchRule) error {
	var (
		v   string
		err error
		kv  = compliance.KVMap{}
	)

	for _, field := range c.audit.Report {
		if c.setStaticKV(field, kv) {
			continue
		}

		switch field.Kind {
		case compliance.PropertyKindAttribute:
			v, err = c.getAttribute(field.Property, r)
		default:
			return ErrPropertyKindNotSupported
		}
		if err != nil {
			return err
		}

		key := field.As
		if key == "" {
			key = field.Property
		}

		if v != "" {
			kv[key] = v
		}
	}

	c.report(nil, kv)
	return nil
}

func (c *auditCheck) getAttribute(name string, r *rule.FileWatchRule) (string, error) {
	switch name {
	case "path":
		return c.audit.Path, nil
	case "enabled":
		return fmt.Sprintf("%t", r != nil), nil
	case "permissions":
		if r == nil {
			return "", nil
		}
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
		return permissions, nil
	default:
		return "", ErrPropertyNotSupported
	}
}
