// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quota

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const (
	supportedSchemaVersion = 1
	actionDrop             = "drop"
)

type payload struct {
	SchemaVersion int        `json:"schema_version"`
	QuotaID       string     `json:"quota_id"`
	Version       int64      `json:"version"`
	Action        string     `json:"action"`
	Match         matchSpec  `json:"match"`
	EnforceUntil  *time.Time `json:"enforce_until"`
	MaxTTLSeconds int64      `json:"max_ttl_seconds"`
	IssuedAt      *time.Time `json:"issued_at"`
	Display       struct {
		Name string `json:"name"`
	} `json:"display"`
}

type matchSpec struct {
	All []clause `json:"all"`
	Any []clause `json:"any"`
}

type clause struct {
	Field  string   `json:"field"`
	Op     string   `json:"op"`
	Value  string   `json:"value"`
	Values []string `json:"values"`
}

type field int

const (
	fieldSource field = iota
	fieldService
	fieldStatus
	fieldTag
	fieldHostTag
)

var fieldsByName = map[string]field{
	"source":   fieldSource,
	"service":  fieldService,
	"status":   fieldStatus,
	"tag":      fieldTag,
	"host_tag": fieldHostTag,
}

type predicate struct {
	field  field
	values []string
}

func (p predicate) matchesMessage(msg *message.Message) bool {
	switch p.field {
	case fieldSource:
		return msg.Origin != nil && slices.Contains(p.values, msg.Origin.Source())
	case fieldService:
		return msg.Origin != nil && slices.Contains(p.values, msg.Origin.Service())
	case fieldStatus:
		return slices.Contains(p.values, msg.GetStatus())
	case fieldTag:
		return msg.Origin != nil && containsAny(msg.Origin.Tags(), p.values)
	}
	return false
}

func containsAny(have, want []string) bool {
	for _, w := range want {
		if slices.Contains(have, w) {
			return true
		}
	}
	return false
}

func (p predicate) String() string {
	name := ""
	for k, v := range fieldsByName {
		if v == p.field {
			name = k
		}
	}
	return name + " in [" + strings.Join(p.values, ",") + "]"
}

// quota is a compiled LOGS_QUOTA rule.
type quota struct {
	id         string
	name       string
	version    int64
	issuedAt   time.Time
	receivedAt time.Time
	deadline   time.Time

	all []predicate
	any []predicate
	// anySatisfied is set when a host_tag clause in "any" matched this host.
	anySatisfied bool
	// hostExcluded is set when host_tag clauses rule this host out entirely.
	hostExcluded bool

	stats *quotaStats
}

func parsePayload(raw []byte) (*payload, error) {
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	if p.SchemaVersion != supportedSchemaVersion {
		return nil, fmt.Errorf("unsupported schema_version %d", p.SchemaVersion)
	}
	if p.QuotaID == "" {
		return nil, errors.New("missing quota_id")
	}
	if p.Action != actionDrop {
		return nil, fmt.Errorf("unsupported action %q", p.Action)
	}
	if p.EnforceUntil == nil && p.MaxTTLSeconds <= 0 {
		return nil, errors.New("one of enforce_until or max_ttl_seconds is required")
	}
	if len(p.Match.All) == 0 && len(p.Match.Any) == 0 {
		return nil, errors.New("match must have at least one clause")
	}
	return &p, nil
}

// compile resolves host_tag clauses up front, since host tags don't vary per message.
func compile(p *payload, receivedAt time.Time, hostTags []string) (*quota, error) {
	q := &quota{
		id:         p.QuotaID,
		name:       p.Display.Name,
		version:    p.Version,
		receivedAt: receivedAt,
	}
	if p.IssuedAt != nil {
		q.issuedAt = *p.IssuedAt
	}

	if p.MaxTTLSeconds > 0 {
		q.deadline = receivedAt.Add(time.Duration(p.MaxTTLSeconds) * time.Second)
	}
	if p.EnforceUntil != nil && (q.deadline.IsZero() || p.EnforceUntil.Before(q.deadline)) {
		q.deadline = *p.EnforceUntil
	}

	for _, c := range p.Match.All {
		pr, err := compileClause(c)
		if err != nil {
			return nil, err
		}
		if pr.field == fieldHostTag {
			if !containsAny(hostTags, pr.values) {
				q.hostExcluded = true
			}
			continue
		}
		q.all = append(q.all, pr)
	}

	for _, c := range p.Match.Any {
		pr, err := compileClause(c)
		if err != nil {
			return nil, err
		}
		if pr.field == fieldHostTag {
			if containsAny(hostTags, pr.values) {
				q.anySatisfied = true
			}
			continue
		}
		q.any = append(q.any, pr)
	}
	if len(p.Match.Any) > 0 && len(q.any) == 0 && !q.anySatisfied {
		q.hostExcluded = true
	}

	return q, nil
}

func compileClause(c clause) (predicate, error) {
	f, ok := fieldsByName[c.Field]
	if !ok {
		return predicate{}, fmt.Errorf("unknown match field %q", c.Field)
	}
	switch c.Op {
	case "eq":
		if c.Value == "" {
			return predicate{}, fmt.Errorf("%s eq: empty value", c.Field)
		}
		return predicate{field: f, values: []string{c.Value}}, nil
	case "in":
		if len(c.Values) == 0 {
			return predicate{}, fmt.Errorf("%s in: empty values", c.Field)
		}
		return predicate{field: f, values: c.Values}, nil
	default:
		return predicate{}, fmt.Errorf("unknown op %q", c.Op)
	}
}

func (q *quota) expired(now time.Time) bool {
	return !now.Before(q.deadline)
}

func (q *quota) matches(msg *message.Message) bool {
	if q.hostExcluded {
		return false
	}
	for _, p := range q.all {
		if !p.matchesMessage(msg) {
			return false
		}
	}
	if len(q.any) == 0 || q.anySatisfied {
		return true
	}
	for _, p := range q.any {
		if p.matchesMessage(msg) {
			return true
		}
	}
	return false
}

func (q *quota) matchString() string {
	var parts []string
	if len(q.all) > 0 {
		s := make([]string, len(q.all))
		for i, p := range q.all {
			s[i] = p.String()
		}
		parts = append(parts, "all("+strings.Join(s, " AND ")+")")
	}
	if len(q.any) > 0 && !q.anySatisfied {
		s := make([]string, len(q.any))
		for i, p := range q.any {
			s[i] = p.String()
		}
		parts = append(parts, "any("+strings.Join(s, " OR ")+")")
	}
	return strings.Join(parts, " AND ")
}
