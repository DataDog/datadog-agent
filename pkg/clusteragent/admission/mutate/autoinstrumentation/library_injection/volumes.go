// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	corev1 "k8s.io/api/core/v1"
)

// Volume and mount path constants shared across injection providers.
const (
	// VolumeName is the name of the main volume used for library injection.
	VolumeName = "datadog-auto-instrumentation"

	// MountPath is the mount path for the library files in application containers.
	MountPath = "/datadog-lib"

	// InjectPackageDir is the path where the APM injector files are stored.
	InjectPackageDir = "opt/datadog-packages/datadog-apm-inject"

	// LibraryPackagesDir is the path where language-specific library files are stored.
	LibraryPackagesDir = "opt/datadog/apm/library"

	// EtcVolumeName is the name of the volume for /etc/ld.so.preload.
	EtcVolumeName = "datadog-auto-instrumentation-etc"

	// InjectorInitContainerName is the name of the APM injector init container.
	InjectorInitContainerName = "datadog-init-apm-inject"
)

// AsAbsPath converts a relative path to an absolute path.
func AsAbsPath(path string) string {
	return "/" + path
}

// InjectorFilePath returns the full path to an injector file.
func InjectorFilePath(name string) string {
	return InjectPackageDir + "/stable/inject/" + name
}

// LibraryInitContainerName returns the init container name for a language.
func LibraryInitContainerName(lang string) string {
	return "datadog-lib-" + lang + "-init"
}

// SupportedLanguages is the list of languages supported for injection.
var SupportedLanguages = []string{
	"java",
	"js",
	"python",
	"dotnet",
	"ruby",
	"php",
}

// IsLanguageSupported checks if a language is supported for injection.
func IsLanguageSupported(lang string) bool {
	for _, l := range SupportedLanguages {
		if l == lang {
			return true
		}
	}
	return false
}

// NewEmptyDirVolume creates an EmptyDir volume with the given name.
func NewEmptyDirVolume(name string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}
