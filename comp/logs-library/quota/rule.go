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

	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type snapshot struct {
	Quotas *[]entry `json:"quotas"`
}

type entry struct {
	Tags      *[]string `json:"tags"`
	ExpiresAt int64     `json:"expires_at"`
}

// quota drops every log carrying all of its lowercase tags, ignoring ASCII case, until its deadline.
type quota struct {
	tags     []string
	key      string
	deadline time.Time
	stats    *quotaStats
}

func parseSnapshot(raw []byte) ([]*quota, error) {
	var s snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	if s.Quotas == nil {
		return nil, errors.New("missing quotas")
	}
	quotas := make([]*quota, 0, len(*s.Quotas))
	for i, e := range *s.Quotas {
		if e.ExpiresAt <= 0 {
			return nil, fmt.Errorf("quotas[%d]: missing expires_at", i)
		}
		if e.Tags == nil {
			return nil, fmt.Errorf("quotas[%d]: missing tags", i)
		}
		tags := make([]string, len(*e.Tags))
		for j, t := range *e.Tags {
			if t == "" {
				return nil, fmt.Errorf("quotas[%d]: empty tag", i)
			}
			tags[j] = asciiLower(t)
		}
		slices.Sort(tags)
		tags = slices.Compact(tags)
		quotas = append(quotas, &quota{
			tags:     tags,
			key:      quotaKey(tags),
			deadline: time.Unix(e.ExpiresAt, 0),
		})
	}
	return quotas, nil
}

func (q *quota) expired(now time.Time) bool {
	return !now.Before(q.deadline)
}

func hasTags(o *message.Origin, tags []string) bool {
	for _, t := range tags {
		if !o.HasTag(t) {
			return false
		}
	}
	return true
}

// retainedBy reports whether f keeps every tag, or its payload attribute, so a log carrying them all still matches.
func retainedBy(f sources.TagFilter, tags []string) bool {
	if f == nil {
		return true
	}
	for _, t := range tags {
		if key, value, ok := strings.Cut(t, ":"); ok && !f.RetainsTag(key, value) && !tagfilter.IsPayloadAttributeKey(key) {
			return false
		}
	}
	return true
}

// quotaKey identifies a quota in stats and telemetry.
func quotaKey(tags []string) string {
	if len(tags) == 0 {
		return "*"
	}
	return strings.Join(tags, ",")
}
