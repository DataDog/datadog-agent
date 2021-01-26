// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package filters

import "github.com/DataDog/datadog-agent/pkg/trace/pb"

// Validator holds a list of required tags.
type Validator struct {
	reqTags []string
}

// Validates returns true if root span contains all required tags from Validator.
func (v *Validator) Validates(span *pb.Span) bool {
	for _, tag := range v.reqTags {
		if _, ok := span.Meta[tag]; !ok {
			return false
		}
	}
	return true
}

// NewValidator creates new Validator based on given list of required tags.
func NewValidator(reqTags []string) *Validator {
	return &Validator{reqTags: reqTags}
}
