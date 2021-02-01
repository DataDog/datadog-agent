// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package filters

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// Validator holds lists of required and rejected tags.
type Validator struct {
	reqTags    []string
	rejectTags []string
}

// Validates returns an error if root span does not contain all required tags and/or contains rejected tags from Validator.
func (v *Validator) Validates(span *pb.Span) error {
	for _, tag := range v.reqTags {
		if _, ok := span.Meta[tag]; !ok {
			return errors.New("required tag(s) missing")
		}
	}
	for _, tag := range v.rejectTags {
		if _, ok := span.Meta[tag]; ok {
			return errors.New("invalid tag(s) found")
		}
	}
	return nil
}

// NewValidator creates new Validator based on given list of required and rejected tags.
func NewValidator(reqTags []string, rejectTags []string) *Validator {
	return &Validator{reqTags: reqTags, rejectTags: rejectTags}
}
