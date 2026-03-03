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
	// InstrumentationVolumeName is the name of the main volume used for APM instrumentation.
	// This volume contains both the APM injector and language-specific library files.
	InstrumentationVolumeName = "datadog-auto-instrumentation"

	// EtcVolumeName is the name of the volume for /etc/ld.so.preload.
	EtcVolumeName = "datadog-auto-instrumentation-etc"

	// libraryMountPath is the mount path for the library files in application containers.
	libraryMountPath = "/datadog-lib"

	// injectPackageDir is the path where the APM injector files are stored.
	injectPackageDir = "opt/datadog-packages/datadog-apm-inject"

	// libraryPackagesDir is the path where language-specific library files are stored.
	libraryPackagesDir = "opt/datadog/apm/library"

	// etcMountPath is the mount path used by init containers to write /etc/ld.so.preload
	// into the shared EmptyDir volume.
	etcMountPath = "/datadog-etc"

	// ldSoPreloadMountPath is where the generated preload file is mounted in app containers.
	ldSoPreloadMountPath = "/etc/ld.so.preload"

	// ldSoPreloadFileName is the filename used inside the shared EmptyDir volume.
	ldSoPreloadFileName = "ld.so.preload"
)

// asAbsPath converts a relative path to an absolute path.
func asAbsPath(path string) string {
	return "/" + path
}

// injectorFilePath returns the full path to an injector file.
func injectorFilePath(name string) string {
	return injectPackageDir + "/stable/inject/" + name
}

// supportedLanguages is the list of languages supported for injection.
var supportedLanguages = []string{
	"java",
	"js",
	"python",
	"dotnet",
	"ruby",
	"php",
}

// IsLanguageSupported checks if a language is supported for injection.
func IsLanguageSupported(lang string) bool {
	for _, l := range supportedLanguages {
		if l == lang {
			return true
		}
	}
	return false
}

// newEmptyDirVolume creates an EmptyDir volume with the given name.
func newEmptyDirVolume(name string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// addEtcLdSoPreloadVolumeAndMounts configures an EmptyDir volume that will provide
// /etc/ld.so.preload to application containers via a SubPath mount.
//
// It returns the init-container mount where the file should be written.
func addEtcLdSoPreloadVolumeAndMounts(patcher *PodPatcher) corev1.VolumeMount {
	patcher.AddVolume(newEmptyDirVolume(EtcVolumeName))

	patcher.AddVolumeMount(corev1.VolumeMount{
		Name:      EtcVolumeName,
		MountPath: ldSoPreloadMountPath,
		SubPath:   ldSoPreloadFileName,
		ReadOnly:  true,
	})

	return corev1.VolumeMount{
		Name:      EtcVolumeName,
		MountPath: etcMountPath,
	}
}
