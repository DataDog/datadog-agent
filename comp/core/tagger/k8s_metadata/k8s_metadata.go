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

// AddMetadataAsTags converts name and value into tags based on the metadata as tags configuration and patterns
func AddMetadataAsTags(name, value string, metadataAsTags map[string]string, glob map[string]glob.Glob, tags *taglist.TagList) {
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
			tags.AddAuto(resolveTag(tmpl, name), value)
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

// resolveTag replaces %%label%%, %%annotation%% and %%env%% by their values
func resolveTag(tmpl, label string) string {
	vars := tmplvar.ParseString(tmpl)
	tagName := tmpl
	for _, v := range vars {
		if _, ok := templateVariables[string(v.Name)]; ok {
			tagName = strings.ReplaceAll(tagName, string(v.Raw), label)
			continue
		}
		tagName = strings.ReplaceAll(tagName, string(v.Raw), "")
	}
	return tagName
}
