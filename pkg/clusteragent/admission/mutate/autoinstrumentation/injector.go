// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
)

var sourceVolume = volume{
	Volume: corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	},
}

var v1VolumeMount = sourceVolume.mount(corev1.VolumeMount{
	MountPath: mountPath,
})

var v2VolumeMountInjector = sourceVolume.mount(corev1.VolumeMount{
	MountPath: "/opt/datadog-packages/datadog-apm-inject",
	SubPath:   "opt/datadog-packages/datadog-apm-inject",
})

var v2VolumeMountLibrary = sourceVolume.mount(corev1.VolumeMount{
	MountPath: "/opt/datadog/apm/library",
	SubPath:   "opt/datadog/apm/library",
})

type injector struct {
	image      string
	registry   string
	injected   bool
	injectTime time.Time
}

func (i *injector) initContainer() initContainer {
	var (
		name  = "datadog-init-apm-inject"
		mount = corev1.VolumeMount{
			MountPath: "/datadog-inject",
			SubPath:   v2VolumeMountInjector.SubPath,
			Name:      v2VolumeMountInjector.Name,
		}
		tsFilePath = mount.MountPath + "/c-init-time." + name
	)
	return initContainer{
		Prepend: true,
		Container: corev1.Container{
			Name:    name,
			Image:   i.image,
			Command: []string{"/bin/sh", "-c", "--"},
			Args: []string{
				fmt.Sprintf(
					`cp -r /%s/* %s && echo $(date +%%s) >> %s`,
					mount.SubPath, mount.MountPath, tsFilePath,
				),
			},
			VolumeMounts: []corev1.VolumeMount{mount},
		},
	}
}

func (i *injector) requirements() libRequirement {
	return libRequirement{
		initContainers: []initContainer{i.initContainer()},
		volumes:        []volume{sourceVolume},
		volumeMounts:   []volumeMount{v2VolumeMountInjector.readOnly().prepended()},
		envVars: []envVar{
			{
				key: "LD_PRELOAD",
				valFunc: identityValFunc(
					"/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so",
				),
			},
			{
				key: "DD_INJECT_SENDER_TYPE",
				valFunc: identityValFunc(
					"k8s",
				),
			},
			{
				key:     "DD_INJECT_START_TIME",
				valFunc: identityValFunc(strconv.FormatInt(i.injectTime.Unix(), 10)),
			},
		},
	}
}

type injectorOption func(*injector)

var injectorVersionAnnotationExtractor = annotationExtractor[injectorOption]{
	key: "admission.datadoghq.com/apm-inject.version",
	do:  infallibleFn(injectorWithImageTag),
}

var injectorImageAnnotationExtractor = annotationExtractor[injectorOption]{
	key: "admission.datadoghq.com/apm-inject.custom-image",
	do:  infallibleFn(injectorWithImageName),
}

func injectorWithImageName(name string) injectorOption {
	return func(i *injector) {
		i.image = name
	}
}

func injectorWithImageTag(tag string) injectorOption {
	return func(i *injector) {
		i.image = fmt.Sprintf("%s/apm-inject:%s", i.registry, tag)
	}
}

func newInjector(startTime time.Time, registry, imageTag string, opts ...injectorOption) *injector {
	i := &injector{
		registry:   registry,
		injectTime: startTime,
	}

	for _, opt := range opts {
		opt(i)
	}

	// if the options didn't override the image, we set it.
	if i.image == "" {
		injectorWithImageTag(imageTag)(i)
	}

	return i
}

func (i *injector) podMutator(v version) podMutator {
	return podMutatorFunc(func(pod *corev1.Pod) error {
		if i.injected {
			return nil
		}

		if !v.usesInjector() {
			return nil
		}

		if err := i.requirements().injectPod(pod, ""); err != nil {
			return err
		}

		i.injected = true
		return nil
	})
}
