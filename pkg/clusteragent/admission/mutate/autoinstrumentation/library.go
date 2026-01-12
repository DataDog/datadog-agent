// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	JavaDefaultLibrary       = NewLibrary(Java, JavaDefaultVersion)
	DotnetDefaultLibrary     = NewLibrary(Dotnet, DotnetDefaultVersion)
	PythonDefaultLibrary     = NewLibrary(Python, PythonDefaultVersion)
	RubyDefaultLibrary       = NewLibrary(Ruby, RubyDefaultVersion)
	JavascriptDefaultLibrary = NewLibrary(Javascript, JavascriptDefaultVersion)
	PHPDefaultLibrary        = NewLibrary(PHP, PHPDefaultVersion)
)

var DefaultLibraries = []Library{
	JavaDefaultLibrary,
	JavascriptDefaultLibrary,
	PythonDefaultLibrary,
	DotnetDefaultLibrary,
	RubyDefaultLibrary,
	PHPDefaultLibrary,
}

var DefaultLibrariesMap = map[Language]Library{
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
	Version         string
	customImage     *Image
	TargetContainer string
}

// NewLibrary will initialize a library and validate the language and version provided.
func NewLibrary(lang Language, version string) Library {
	// Get the default version.
	defaultVersion := DefaultVersions[lang]

	// Set the version to the default if there isn't one provided or the magic string is provided.
	if version == "" || version == DefaultVersionMagicString {
		version = defaultVersion
	}

	// Return the library.
	return Library{
		Language: lang,
		Version:  version,
	}
}

func NewTargetedLibrary(lang Language, version string, targetContainer string) Library {
	lib := NewLibrary(lang, version)
	lib.TargetContainer = targetContainer
	return lib
}

func NewTargetedLibraryFromImage(lang Language, image *Image, targetContainer string) Library {
	return Library{
		Language:        lang,
		Version:         image.Tag,
		customImage:     image,
		TargetContainer: targetContainer,
	}
}

func NewLibraryFromImage(lang Language, image *Image) Library {
	return Library{
		Language:    lang,
		Version:     image.Tag,
		customImage: image,
	}
}

func (l Library) IsDefault() bool {
	return l.Version == DefaultVersions[l.Language]
}

func (l Library) LibInfo(registry string, ctrName string) libInfo {
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
func (l Library) InitImage(registry string) *Image {
	if l.customImage != nil {
		return l.customImage
	}

	return &Image{
		Registry: registry,
		Name:     "dd-lib-" + string(l.Language) + "-init",
		Tag:      l.Version,
	}
}

func ExtractLibrariesFromAnnotations(pod *corev1.Pod, registry string) []Library {
	libs := []Library{}

	// Check all supported languages for potential Local SDK Injection.
	for _, l := range SupportedLanguages {
		// Check for a custom library image.
		customImage, found := GetAnnotation(pod, AnnotationLibraryImage.Format(string(l)))
		if found {
			image, err := NewImage(customImage)
			if err != nil {
				log.Warnf("could not extract custom image: %w", err)
			} else {
				libs = append(libs, NewLibraryFromImage(l, image))
			}
		}

		// Check for a custom library version.
		libVersion, found := GetAnnotation(pod, AnnotationLibraryVersion.Format(string(l)))
		if found {
			libs = append(libs, NewLibrary(l, libVersion))
		}

		// Check all containers in the pod for container specific Local SDK Injection.
		for _, container := range pod.Spec.Containers {
			// Check for custom library image.
			customImage, found := GetAnnotation(pod, AnnotationLibraryContainerImage.Format(container.Name, string(l)))
			if found {
				image, err := NewImage(customImage)
				if err != nil {
					log.Warnf("could not extract custom image: %w", err)
				} else {
					libs = append(libs, NewTargetedLibraryFromImage(l, image, container.Name))
				}
			}

			// Check for custom library version.
			libVersion, found := GetAnnotation(pod, AnnotationLibraryContainerVersion.Format(container.Name, string(l)))
			if found {
				libs = append(libs, NewTargetedLibrary(l, libVersion, container.Name))
			}
		}
	}

	return libs
}

func ParseLibraries(libVersions map[string]string) []Library {
	libs := []Library{}

	for lang, version := range libVersions {
		l, err := NewLanguage(lang)
		if err != nil {
			log.Warnf("APM Instrumentation detected configuration for unsupported language: %s. Tracing library for %s will not be injected", lang, lang)
			continue
		}

		libs = append(libs, NewLibrary(l, version))
	}

	return libs
}

func AreDefaults(libs []Library) bool {
	for _, lib := range libs {
		if !lib.IsDefault() {
			return false
		}
	}

	return true
}
