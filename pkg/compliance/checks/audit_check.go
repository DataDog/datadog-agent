// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/elastic/go-libaudit/rule"
)

type auditCheck struct {
	baseCheck
	audit *compliance.Audit
}

func newAuditCheck(baseCheck baseCheck, audit *compliance.Audit) (*auditCheck, error) {
	if err := audit.Validate(); err != nil {
		return nil, fmt.Errorf("unable to create audit check for invalid audit resource %w", err)
	}

	return &auditCheck{
		baseCheck: baseCheck,
		audit:     audit,
	}, nil
}

func (c *auditCheck) Run() error {
	log.Debugf("%s: running audit check - path %s", c.id, c.audit.Path)

	rules, err := c.AuditClient().GetFileWatchRules()
	if err != nil {
		return err
	}

	path := c.audit.Path
	if path == "" {
		path, err = c.ResolveValueFrom(c.audit.PathFrom)
		if err != nil {
			return err
		}
	}

	// Scan for the rule matching configured path
	for _, r := range rules {
		if r.Path == path {
			log.Debugf("%s: audit check - match %s", c.id, path)
			return c.reportOnRule(r, path)
		}
	}

	// If no rule found we still report this as "not enabled"
	return c.reportOnRule(nil, path)
}

func (c *auditCheck) reportOnRule(r *rule.FileWatchRule, path string) error {
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
			v, err = c.getAttribute(field.Property, r, path)
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

func (c *auditCheck) getAttribute(name string, r *rule.FileWatchRule, path string) (string, error) {
	switch name {
	case "path":
		return path, nil
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
