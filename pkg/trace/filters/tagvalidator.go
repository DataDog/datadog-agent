// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package filters

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// TagValidator holds lists of required and rejected tags and values.
type TagValidator struct {
	reqTags    map[string]string
	rejectTags map[string]string
}

// Validates returns an error if root span does not contain all required tags and/or contains rejected tags.
func (tv *TagValidator) Validates(span *pb.Span) error {
	for tag := range tv.reqTags {
		if _, ok := span.Meta[tag]; !ok {
			return errors.New("required tag(s) missing")
		}
	}
	for tag, v := range tv.rejectTags {
		if v == span.Meta[tag] {
			return errors.New("invalid tag(s) found")
		}
	}
	return nil
}

// NewTagValidator creates new Validator based on given list of required and rejected tags.
func NewTagValidator(reqTags map[string]string, rejectTags map[string]string) *TagValidator {
	return &TagValidator{reqTags: reqTags, rejectTags: rejectTags}
}
