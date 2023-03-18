// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tags

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// newTagListBuilder returns a tagListBuilder.
func newTagListBuilder() *tagListBuilder {
	return &tagListBuilder{
		sb:      strings.Builder{},
		tagList: []string{},
	}
}

type tagListBuilder struct {
	sb      strings.Builder
	tagList []string
}

// addNotEmpty builds and appends non-empty tags.
// It discards the tag if one of the arguments is empty.
func (tlb *tagListBuilder) addNotEmpty(k, v string) {
	if tag := tlb.buildTag(k, v); len(tag) > 0 {
		tlb.tagList = append(tlb.tagList, tag)
	}
}

// tags returns the added tags.
func (tlb *tagListBuilder) tags() []string {
	return tlb.tagList
}

// buildTag returns converts the arguments into a tag string k:v
// It returns an empty string if one of the arguments is empty.
func (tlb *tagListBuilder) buildTag(k, v string) string {
	if k == "" || v == "" {
		log.Debugf("Cannot build tag with empty key or value: key %q - value %q", k, v)
		return ""
	}

	defer tlb.sb.Reset()
	tlb.sb.WriteString(k)
	tlb.sb.WriteString(":")
	tlb.sb.WriteString(v)

	return tlb.sb.String()
}
