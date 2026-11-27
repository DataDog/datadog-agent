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

type snapshot struct {
	Quotas *[]entry `json:"quotas"`
}

type entry struct {
	Tags      []string `json:"tags"`
	ExpiresAt int64    `json:"expires_at"`
}

// quota drops every log carrying all of its tags until its deadline.
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
		if slices.Contains(e.Tags, "") {
			return nil, fmt.Errorf("quotas[%d]: empty tag", i)
		}
		tags := slices.Compact(slices.Sorted(slices.Values(e.Tags)))
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

func (q *quota) matches(o *message.Origin) bool {
	for _, t := range q.tags {
		if !o.HasTag(t) {
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
