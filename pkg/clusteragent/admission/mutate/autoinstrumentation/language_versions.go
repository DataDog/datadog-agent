// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"
	"slices"
)

const (
	java   language = "java"
	js     language = "js"
	python language = "python"
	dotnet language = "dotnet"
	ruby   language = "ruby"
	php    language = "php"
)

// language is lang-library we might be injecting.
type language string

func (l language) defaultLibInfo(registry, ctrName string) libInfo {
	return l.libInfoWithResolver(ctrName, registry, l.defaultLibVersion())
}

// DEV: This is just formatting, no resolution is done here
func (l language) libImageName(registry, tag string) string {
	if tag == defaultVersionMagicString {
		tag = l.defaultLibVersion()
	}

	return fmt.Sprintf("%s/dd-lib-%s-init:%s", registry, l, tag)
}

// DEV: Legacy
func (l language) libInfo(ctrName, image string) libInfo {
	return libInfo{
		lang:    l,
		ctrName: ctrName,
		image:   image,
	}
}

// DEV: Will attempt to resolve, defaults to legacy if unable
func (l language) libInfoWithResolver(ctrName, registry string, version string) libInfo {
	if version == defaultVersionMagicString {
		version = l.defaultLibVersion()
	}

	return libInfo{
		lang:       l,
		ctrName:    ctrName,
		image:      l.libImageName(registry, version),
		registry:   registry,
		repository: fmt.Sprintf("dd-lib-%s-init", l),
		tag:        version,
	}
}

// supportedLanguages defines a list of the languages that we will attempt
// to do injection on.
var supportedLanguages = []language{
	java,
	js,
	python,
	dotnet,
	ruby,
	php, // PHP only works with injection v2, no environment variables are set in any case
}

func defaultSupportedLanguagesMap() map[language]bool {
	m := map[language]bool{}
	for _, l := range supportedLanguages {
		m[l] = true
	}

	return m
}

func (l language) isSupported() bool {
	return slices.Contains(supportedLanguages, l)
}

// defaultVersionMagicString is a magic string that indicates that the user
// wishes to utilize the default version found in languageVersions.
const defaultVersionMagicString = "default"

// languageVersions defines the major library versions we consider "default" for each
// supported language. If not set, we will default to "latest", see defaultLibVersion.
//
// If this language does not appear in supportedLanguages, it will not be injected.
var languageVersions = map[language]string{
	java:   "v1", // https://datadoghq.atlassian.net/browse/APMON-1064
	dotnet: "v3", // https://datadoghq.atlassian.net/browse/APMON-1390
	python: "v4", // https://datadoghq.atlassian.net/browse/INPLAT-852
	ruby:   "v2", // https://datadoghq.atlassian.net/browse/APMON-1066
	js:     "v5", // https://datadoghq.atlassian.net/browse/APMON-1065
	php:    "v1", // https://datadoghq.atlassian.net/browse/APMON-1128
}

func (l language) defaultLibVersion() string {
	langVersion, ok := languageVersions[l]
	if !ok {
		return "latest"
	}
	return langVersion
}

type libInfo struct {
	ctrName    string // empty means all containers
	lang       language
	image      string
	registry   string
	repository string
	tag        string
}

func initContainerName(lang language) string {
	return fmt.Sprintf("datadog-lib-%s-init", lang)
}
