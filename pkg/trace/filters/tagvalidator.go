// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package filters

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// TagValidator holds a list of TagRules which contains required or rejected tags with specified keys and values.
type TagValidator struct {
	list []*config.TagRule
}

// NewTagValidator creates new TagValidator based on a given list of TagRules.
func NewTagValidator(list []*config.TagRule) *TagValidator {
	return &TagValidator{list: list}
}

// Validates returns the first error if root span does not contain all required tags and/or contains rejected tags.
func (tv *TagValidator) Validates(span *pb.Span) error {
	for _, tag := range tv.list {
		v, ok := span.Meta[tag.Name]
		if tag.Type == 1 {
			if !ok || (tag.Value != "" && v != "" && v != tag.Value) {
				return errors.New("required tag(s) missing")
			}
		}
		if tag.Type == 0 {
			if ok || (tag.Value != "" && v != "" && v == tag.Value) {
				return errors.New("invalid tag(s) found")
			}
		}
	}
	return nil
}
