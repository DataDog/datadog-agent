// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type libInfo struct {
	ctrName          string // empty means all containers
	lang             Language
	image            string
	canonicalVersion string
	registry         string
	repository       string
	tag              string
}

func newLibInfo(lib Library, registry string) libInfo {
	img := lib.InitImage(registry)
	return libInfo{
		ctrName:          lib.TargetContainer,
		lang:             lib.Language,
		image:            img.String(),
		canonicalVersion: img.Tag,
		registry:         img.Registry,
		repository:       img.Name,
	}
}

func (i *libInfo) podMutator(opts libRequirementOptions, imageResolver ImageResolver) podMutator {
	return podMutatorFunc(func(pod *corev1.Pod) error {
		reqs, ok := i.libRequirement(imageResolver)
		if !ok {
			return fmt.Errorf(
				"language %q is not supported. Supported languages are %v",
				i.lang, SupportedLanguages,
			)
		}

		reqs.libRequirementOptions = opts

		if i.canonicalVersion != "" {
			SetAnnotation(pod, AnnotationLibraryCanonicalVersion.Format(string(i.lang)), i.canonicalVersion)
		}

		if err := reqs.injectPod(pod, i.ctrName); err != nil {
			return err
		}

		return nil
	})
}

// initContainers is which initContainers we are injecting
// into the pod that runs for this language.
func (i *libInfo) initContainers(resolver ImageResolver) []initContainer {
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
			i.canonicalVersion = image.CanonicalVersion
		} else {
			i.canonicalVersion = ""
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

func (i *libInfo) volumeMount() volumeMount {
	return v2VolumeMountLibrary
}

func (i *libInfo) libRequirement(resolver ImageResolver) (libRequirement, bool) {
	if !i.lang.IsSupported() {
		return libRequirement{}, false
	}

	return libRequirement{
		initContainers: i.initContainers(resolver),
		volumeMounts:   []volumeMount{i.volumeMount()},
		volumes:        []volume{sourceVolume},
	}, true
}

func initContainerName(lang Language) string {
	return fmt.Sprintf("datadog-lib-%s-init", lang)
}
