// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
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
	return l.libInfo(ctrName, l.libImageName(registry, l.defaultLibVersion()))
}

func (l language) libImageName(registry, tag string) string {
	switch tag {
	case "latest":
		// do nothing with this as a shortcut well known valid tag.
	case defaultVersionMagicString:
		tag = l.defaultLibVersion()
	default:
		if l.isDefaultVersionMoreSpecificThan(tag) {
			tag = l.defaultLibVersion()
		}
	}

	return fmt.Sprintf("%s/dd-lib-%s-init:%s", registry, l, tag)
}

func (l language) isDefaultVersionMoreSpecificThan(input string) bool {
	v, ok := languageVersions[l]
	if !ok {
		return false
	}

	in, err := newLibraryVersion(input)
	if err != nil {
		return false
	}

	return isDefaultVersionMoreSpecific(v, in)
}

// isDefaultVersionMoreSpecific tells us whether or not we should use
// the default version even though a user-specified one might be set.
//
// For example if we have `v1.46` as the defaultVersion, a user setting
// `v1` should have us using `v1.46` but them specifying a more specific
// point release would leave that alone.
func isDefaultVersionMoreSpecific(s1, s2 *libraryVersion) bool {
	// we only care if there is a minor version specified
	// in the `inputVersion` and the majors match since our `languageVersions`
	// will only be as specific as the minor.
	return s1.major == s2.major && s2.segmentsSpecified == 1
}

func (l language) libInfo(ctrName, image string) libInfo {
	return libInfo{
		lang:    l,
		ctrName: ctrName,
		image:   image,
	}
}

const (
	libVersionAnnotationKeyFormat    = "admission.datadoghq.com/%s-lib.version"
	customLibAnnotationKeyFormat     = "admission.datadoghq.com/%s-lib.custom-image"
	libVersionAnnotationKeyCtrFormat = "admission.datadoghq.com/%s.%s-lib.version"
	customLibAnnotationKeyCtrFormat  = "admission.datadoghq.com/%s.%s-lib.custom-image"
)

func (l language) customLibAnnotationExtractor() annotationExtractor[libInfo] {
	return annotationExtractor[libInfo]{
		key: fmt.Sprintf(customLibAnnotationKeyFormat, l),
		do: func(image string) (libInfo, error) {
			return l.libInfo("", image), nil
		},
	}
}

func (l language) libVersionAnnotationExtractor(registry string) annotationExtractor[libInfo] {
	return annotationExtractor[libInfo]{
		key: fmt.Sprintf(libVersionAnnotationKeyFormat, l),
		do: func(version string) (libInfo, error) {
			return l.libInfo("", l.libImageName(registry, version)), nil
		},
	}
}

func (l language) ctrCustomLibAnnotationExtractor(ctr string) annotationExtractor[libInfo] {
	return annotationExtractor[libInfo]{
		key: fmt.Sprintf(customLibAnnotationKeyCtrFormat, ctr, l),
		do: func(image string) (libInfo, error) {
			return l.libInfo(ctr, image), nil
		},
	}
}

func (l language) ctrLibVersionAnnotationExtractor(ctr, registry string) annotationExtractor[libInfo] {
	return annotationExtractor[libInfo]{
		key: fmt.Sprintf(libVersionAnnotationKeyCtrFormat, ctr, l),
		do: func(version string) (libInfo, error) {
			return l.libInfo(ctr, l.libImageName(registry, version)), nil
		},
	}
}

func (l language) libConfigAnnotationExtractor() annotationExtractor[common.LibConfig] {
	return annotationExtractor[common.LibConfig]{
		key: fmt.Sprintf(common.LibConfigV1AnnotKeyFormat, l),
		do:  parseConfigJSON,
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

func (l language) isSupported() bool {
	return slices.Contains(supportedLanguages, l)
}

func (l language) isEnabledByDefault() bool {
	return l != "php"
}

// defaultVersionMagicString is a magic string that indicates that the user
// wishes to utilize the default version found in languageVersions.
const defaultVersionMagicString = "default"

type libraryVersion struct {
	major, minor      int64
	segmentsSpecified int
	original          string
}

func newLibraryVersion(in string) (*libraryVersion, error) {
	noV, _ := strings.CutPrefix(strings.TrimSpace(in), "v")
	split := strings.Split(noV, ".")

	var major, minor int64
outer:
	for i, v := range split {
		val, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing version: %w", err)
		}

		switch i {
		case 0:
			major = val
		case 1:
			minor = val
			break outer
		}
	}

	return &libraryVersion{
		major:             major,
		minor:             minor,
		segmentsSpecified: len(split),
		original:          in,
	}, nil
}

func mustLibraryVersion(in string) *libraryVersion {
	v, err := newLibraryVersion(in)
	if err != nil {
		panic(err)
	}
	return v
}

// languageVersions defines the major library versions we consider "default" for each
// supported language. If not set, we will default to "latest", see defaultLibVersion.
//
// If this language does not appear in supportedLanguages, it will not be injected.
var languageVersions = map[language]*libraryVersion{
	java:   mustLibraryVersion("v1.46"), // https://datadoghq.atlassian.net/browse/APMON-1064
	dotnet: mustLibraryVersion("v3.10"), // https://datadoghq.atlassian.net/browse/APMON-1390
	python: mustLibraryVersion("v2.21"), // https://datadoghq.atlassian.net/browse/APMON-1068
	ruby:   mustLibraryVersion("v2.10"), // https://datadoghq.atlassian.net/browse/APMON-1066
	js:     mustLibraryVersion("v5.37"), // https://datadoghq.atlassian.net/browse/APMON-1065
	php:    mustLibraryVersion("v1.6"),  // https://datadoghq.atlassian.net/browse/APMON-1128
}

func (l language) defaultLibVersion() string {
	langVersion, ok := languageVersions[l]
	if !ok {
		return "latest"
	}

	return langVersion.original
}

type libInfo struct {
	ctrName string // empty means all containers
	lang    language
	image   string
}

func (i libInfo) podMutator(v version, opts libRequirementOptions) podMutator {
	return podMutatorFunc(func(pod *corev1.Pod) error {
		reqs, ok := i.libRequirement(v)
		if !ok {
			return fmt.Errorf(
				"language %q is not supported. Supported languages are %v",
				i.lang, supportedLanguages,
			)
		}

		reqs.libRequirementOptions = opts

		if err := reqs.injectPod(pod, i.ctrName); err != nil {
			return err
		}

		return nil
	})
}

// initContainers is which initContainers we are injecting
// into the pod that runs for this language.
func (i libInfo) initContainers(v version) []initContainer {
	var (
		args, command []string
		mounts        []corev1.VolumeMount
		cName         = initContainerName(i.lang)
	)

	if v.usesInjector() {
		mounts = []corev1.VolumeMount{
			// we use the library mount on its lang-based sub-path
			{
				MountPath: v1VolumeMount.MountPath,
				SubPath:   v2VolumeMountLibrary.SubPath + "/" + string(i.lang),
				Name:      sourceVolume.Name,
			},
			// injector mount for the timestamps
			v2VolumeMountInjector.VolumeMount,
		}
		tsFilePath := v2VolumeMountInjector.MountPath + "/c-init-time." + cName
		command = []string{"/bin/sh", "-c", "--"}
		args = []string{
			fmt.Sprintf(
				`sh copy-lib.sh %s && echo $(date +%%s) >> %s`,
				mounts[0].MountPath, tsFilePath,
			),
		}
	} else {
		mounts = []corev1.VolumeMount{v1VolumeMount.VolumeMount}
		command = []string{"sh", "copy-lib.sh", mounts[0].MountPath}
	}

	return []initContainer{
		{
			Container: corev1.Container{
				Name:         cName,
				Image:        i.image,
				Command:      command,
				Args:         args,
				VolumeMounts: mounts,
			},
		},
	}
}

func (i libInfo) volumeMount(v version) volumeMount {
	if v.usesInjector() {
		return v2VolumeMountLibrary
	}

	return v1VolumeMount
}

func (i libInfo) envVars(v version) []envVar {
	if v.usesInjector() {
		return nil
	}

	switch i.lang {
	case java:
		return []envVar{
			{
				key:     javaToolOptionsKey,
				valFunc: javaEnvValFunc,
			},
		}
	case js:
		return []envVar{
			{
				key:     nodeOptionsKey,
				valFunc: jsEnvValFunc,
			},
		}
	case python:
		return []envVar{
			{
				key:     pythonPathKey,
				valFunc: pythonEnvValFunc,
			},
		}
	case dotnet:
		return []envVar{
			{
				key:     dotnetClrEnableProfilingKey,
				valFunc: identityValFunc(dotnetClrEnableProfilingValue),
			},
			{
				key:     dotnetClrProfilerIDKey,
				valFunc: identityValFunc(dotnetClrProfilerIDValue),
			},
			{
				key:     dotnetClrProfilerPathKey,
				valFunc: identityValFunc(dotnetClrProfilerPathValue),
			},
			{
				key:     dotnetTracerHomeKey,
				valFunc: identityValFunc(dotnetTracerHomeValue),
			},
			{
				key:     dotnetTracerLogDirectoryKey,
				valFunc: identityValFunc(dotnetTracerLogDirectoryValue),
			},
			{
				key:     dotnetProfilingLdPreloadKey,
				valFunc: dotnetProfilingLdPreloadEnvValFunc,
				isEligibleToInject: func(_ *corev1.Container) bool {
					// N.B. Always disabled for now until we have a better mechanism to inject
					//      this safely.
					return false
				},
			},
		}
	case ruby:
		return []envVar{
			{
				key:     rubyOptKey,
				valFunc: rubyEnvValFunc,
			},
		}
	default:
		return nil
	}
}

func (i libInfo) libRequirement(v version) (libRequirement, bool) {
	if !i.lang.isSupported() {
		return libRequirement{}, false
	}

	return libRequirement{
		envVars:        i.envVars(v),
		initContainers: i.initContainers(v),
		volumeMounts:   []volumeMount{i.volumeMount(v)},
		volumes:        []volume{sourceVolume},
	}, true
}
