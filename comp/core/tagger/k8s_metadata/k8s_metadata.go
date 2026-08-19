// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package k8smetadata provides utilities to handle kubernetes metadata as tags
package k8smetadata

import (
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/tmplvar"

	"github.com/gobwas/glob"
)

// InitMetadataAsTags prepares labels and annotations as tags
// - It lower-case all the keys in metadataAsTags
// - It compiles all the patterns and stores them in a map of glob.Glob objects
func InitMetadataAsTags(metadataAsTags map[string]string) (map[string]string, map[string]glob.Glob) {
	// We lower-case the values collected by viper as well as the ones from inspecting the pod labels/annotations.
	globMap := map[string]glob.Glob{}
	for metadataKey, value := range metadataAsTags {
		delete(metadataAsTags, metadataKey)
		pattern := strings.ToLower(metadataKey)
		metadataAsTags[pattern] = value
		if strings.Contains(pattern, "*") {
			g, err := glob.Compile(pattern)
			if err != nil {
				log.Errorf("Failed to compile glob for [%s]: %v", pattern, err)
				continue
			}
			globMap[pattern] = g
		}
	}
	return metadataAsTags, globMap
}

// AddMetadataAsTags converts name and value into tags based on the metadata as tags configuration and patterns.
// Use it for metadata the cluster administrator controls, such as node or namespace labels.
func AddMetadataAsTags(name, value string, metadataAsTags map[string]string, glob map[string]glob.Glob, tags *taglist.TagList) {
	addMetadataAsTags(name, value, metadataAsTags, glob, tags, false)
}

// AddWorkloadMetadataAsTags is AddMetadataAsTags for metadata the workload itself controls, such as
// pod labels and annotations or container labels and environment variables. A tag whose name is
// derived from the metadata name through a %%label%%, %%annotation%% or %%env%% template is dropped
// when that name is denied, so that a workload cannot name its own tags after the reserved ones.
// Tag names the administrator hardcoded in the mapping are kept as-is.
func AddWorkloadMetadataAsTags(name, value string, metadataAsTags map[string]string, glob map[string]glob.Glob, tags *taglist.TagList) {
	addMetadataAsTags(name, value, metadataAsTags, glob, tags, true)
}

func addMetadataAsTags(name, value string, metadataAsTags map[string]string, glob map[string]glob.Glob, tags *taglist.TagList, fromWorkload bool) {
	for pattern, tmplStr := range metadataAsTags {
		n := strings.ToLower(name)
		if g, ok := glob[pattern]; ok {
			if !g.Match(n) {
				continue
			}
		} else if pattern != n {
			continue
		}
		tagTmplList := splitTags(tmplStr)
		for _, tmpl := range tagTmplList {
			tagName, fromMetadata := resolveTag(tmpl, name)
			if fromWorkload && fromMetadata {
				tags.AddAutoFromWorkload(tagName, value)
				continue
			}
			tags.AddAuto(tagName, value)
		}
	}
}

// splitTags splits tmplStr into tag slice using "," as delimiter. This can generate multiple tags from a label
func splitTags(tmplStr string) []string {
	tagTmpList := strings.Split(tmplStr, ",")
	for i := range tagTmpList {
		tagTmpList[i] = strings.TrimSpace(tagTmpList[i])
	}
	return tagTmpList
}

var templateVariables = map[string]struct{}{
	"label":      {},
	"annotation": {},
	"env":        {},
}

// resolveTag replaces %%label%%, %%annotation%% and %%env%% by their values.
// It also reports whether the resulting tag name embeds the metadata name,
// meaning the workload has a say in how the tag is named.
func resolveTag(tmpl, label string) (string, bool) {
	vars := tmplvar.ParseString(tmpl)
	tagName := tmpl
	fromMetadata := false
	for _, v := range vars {
		if _, ok := templateVariables[string(v.Name)]; ok {
			tagName = strings.ReplaceAll(tagName, string(v.Raw), label)
			fromMetadata = true
			continue
		}
		tagName = strings.ReplaceAll(tagName, string(v.Raw), "")
	}
	return tagName, fromMetadata
}
