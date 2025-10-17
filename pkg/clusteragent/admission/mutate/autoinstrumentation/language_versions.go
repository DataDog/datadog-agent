// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
			return l.libInfoWithResolver("", registry, version), nil
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
			return l.libInfoWithResolver(ctr, registry, version), nil
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
	python: "v3", // https://datadoghq.atlassian.net/browse/INPLAT-598
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

func (i libInfo) podMutator(opts libRequirementOptions, imageResolver ImageResolver) podMutator {
	return podMutatorFunc(func(pod *corev1.Pod) error {
		reqs, ok := i.libRequirement(imageResolver)
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
func (i libInfo) initContainers(resolver ImageResolver) []initContainer {
	var (
		args, command []string
		mounts        []corev1.VolumeMount
		cName         = initContainerName(i.lang)
	)

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

	if resolver != nil {
		log.Debugf("Resolving image %s/%s:%s", i.registry, i.repository, i.tag)
		image, ok := resolver.Resolve(i.registry, i.repository, i.tag)
		if ok {
			i.image = image.FullImageRef
		}
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

func (i libInfo) volumeMount() volumeMount {
	return v2VolumeMountLibrary
}

func (i libInfo) libRequirement(resolver ImageResolver) (libRequirement, bool) {
	if !i.lang.isSupported() {
		return libRequirement{}, false
	}

	return libRequirement{
		initContainers: i.initContainers(resolver),
		volumeMounts:   []volumeMount{i.volumeMount()},
		volumes:        []volume{sourceVolume},
	}, true
}
