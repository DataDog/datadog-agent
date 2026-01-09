// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

var (
	JavaDefaultLibrary       = NewLibrary(Java, JavaDefaultVersion)
	DotnetDefaultLibrary     = NewLibrary(Dotnet, DotnetDefaultVersion)
	PythonDefaultLibrary     = NewLibrary(Python, PythonDefaultVersion)
	RubyDefaultLibrary       = NewLibrary(Ruby, RubyDefaultVersion)
	JavascriptDefaultLibrary = NewLibrary(Javascript, JavascriptDefaultVersion)
	PHPDefaultLibrary        = NewLibrary(PHP, PHPDefaultVersion)
)

var DefaultLibraries = map[Language]*Library{
	Java:       JavaDefaultLibrary,
	Dotnet:     DotnetDefaultLibrary,
	Python:     PythonDefaultLibrary,
	Ruby:       RubyDefaultLibrary,
	Javascript: JavascriptDefaultLibrary,
	PHP:        PHPDefaultLibrary,
}

// Library is a type to represent an SDK that can be delivered through Single Step Instrumentation. It is a language
// version combo.
type Library struct {
	// Language is the language the SDK supports.
	Language Language
	// Version is the version of the SDK.
	Version string
}

// NewLibrary will initialize a library and validate the language and version provided.
func NewLibrary(lang Language, version string) *Library {
	// Get the default version.
	defaultVersion := DefaultVersions[lang]

	// Set the version to the default if there isn't one provided or the magic string is provided.
	if version == "" || version == DefaultVersionMagicString {
		version = defaultVersion
	}

	// Return the library.
	return &Library{
		Language: lang,
		Version:  version,
	}
}

func (l *Library) IsDefault() bool {
	return l.Version == DefaultVersions[l.Language]
}

func (l *Library) LibInfo(registry string, ctrName string) libInfo {
	img := l.InitImage(registry)
	return libInfo{
		lang:       l.Language,
		ctrName:    ctrName,
		image:      img.String(),
		registry:   img.Registry,
		repository: img.Name,
		tag:        img.Tag,
	}
}

// InitImage will return a populated container image that can be used to pull a copy of the SDK.
func (l *Library) InitImage(registry string) *Image {
	return &Image{
		Registry: registry,
		Name:     "dd-lib-" + string(l.Language) + "-init",
		Tag:      l.Version,
	}
}
