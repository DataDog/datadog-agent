// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"github.com/gobwas/glob"
)

// InitMetadataAsTags prepares labels and annotations as tags
// - It lower-case all the labels in metadataAsTags
// - It compiles all the patterns and stores them in a map of glob.Glob objects
func InitMetadataAsTags(metadataAsTags map[string]string) (map[string]string, map[string]glob.Glob) {
	panic("not called")
}

// AddMetadataAsTags converts name and value into tags based on the metadata as tags configuration and patterns
func AddMetadataAsTags(name, value string, metadataAsTags map[string]string, glob map[string]glob.Glob, tags *TagList) {
	panic("not called")
}

// splitTags splits tmplStr into tag slice using "," as delimiter. This can generate multiple tags from a label
func splitTags(tmplStr string) []string {
	panic("not called")
}

var templateVariables = map[string]struct{}{
	"label":      {},
	"annotation": {},
	"env":        {},
}

// resolveTag replaces %%label%%, %%annotation%% and %%env%% by their values
func resolveTag(tmpl, label string) string {
	panic("not called")
}
