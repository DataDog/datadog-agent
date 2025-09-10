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

	"github.com/DataDog/datadog-agent/pkg/util/log"
	corev1 "k8s.io/api/core/v1"
)

const (
	injectPackageDir   = "opt/datadog-packages/datadog-apm-inject"
	libraryPackagesDir = "opt/datadog/apm/library"
)

func asAbs(path string) string {
	return "/" + path
}

func injectorFilePath(name string) string {
	return injectPackageDir + "/stable/inject/" + name
}

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
	MountPath: asAbs(injectPackageDir),
	SubPath:   injectPackageDir,
})

var v2VolumeMountLibrary = sourceVolume.mount(corev1.VolumeMount{
	MountPath: asAbs(libraryPackagesDir),
	SubPath:   libraryPackagesDir,
})

var etcVolume = volume{
	Volume: corev1.Volume{
		Name: "datadog-auto-instrumentation-etc",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	},
}

var volumeMountETCDPreloadInitContainer = etcVolume.mount(corev1.VolumeMount{
	MountPath: "/datadog-etc",
})

var volumeMountETCDPreloadAppContainer = etcVolume.mount(corev1.VolumeMount{
	MountPath: "/etc/ld.so.preload",
	SubPath:   "ld.so.preload",
	ReadOnly:  true,
})

type injector struct {
	image         string
	registry      string
	debug         bool
	injected      bool
	injectTime    time.Time
	opts          libRequirementOptions
	imageResolver ImageResolver
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
				// TODO: We should probably move this into either a script that's in the container _or_
				//       something we can do with a go template because this is not great.
				fmt.Sprintf(
					`cp -r /%s/* %s && echo %s > /datadog-etc/ld.so.preload && echo $(date +%%s) >> %s`,
					mount.SubPath,
					mount.MountPath,
					asAbs(injectorFilePath("launcher.preload.so")),
					tsFilePath,
				),
			},
			VolumeMounts: []corev1.VolumeMount{
				mount,
				volumeMountETCDPreloadInitContainer.VolumeMount,
			},
		},
	}
}

func (i *injector) requirements() libRequirement {
	return libRequirement{
		libRequirementOptions: i.opts,
		initContainers:        []initContainer{i.initContainer()},
		volumes: []volume{
			sourceVolume,
			etcVolume,
		},
		volumeMounts: []volumeMount{
			volumeMountETCDPreloadAppContainer.prepended(),
			v2VolumeMountInjector.prepended(),
		},
		envVars: append(
			[]envVar{
				{
					key:     "LD_PRELOAD",
					valFunc: joinValFunc(asAbs(injectorFilePath("launcher.preload.so")), ":"),
				},
				{
					key:     "DD_INJECT_SENDER_TYPE",
					valFunc: identityValFunc("k8s"),
				},
				{
					key:     "DD_INJECT_START_TIME",
					valFunc: identityValFunc(strconv.FormatInt(i.injectTime.Unix(), 10)),
				},
			},
			i.debugEnvVars()...,
		),
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

var injectorDebugAnnotationExtractor = annotationExtractor[injectorOption]{
	key: "admission.datadoghq.com/apm-inject.debug",
	do:  infallibleFn(injectorDebug),
}

func injectorWithLibRequirementOptions(opts libRequirementOptions) injectorOption {
	return func(i *injector) {
		i.opts = opts
	}
}

func injectorWithImageName(name string) injectorOption {
	return func(i *injector) {
		i.image = name
	}
}

func injectorWithImageTag(tag string) injectorOption {
	return func(i *injector) {
		if i.imageResolver == nil {
			log.Error("injectorWithImageTag called without imageResolver")
			return
		}
		if resolvedImage, ok := i.imageResolver.Resolve(i.registry, "apm-inject", tag); ok {
			log.Debugf("Resolved image for %s/apm-inject:%s: %s", i.registry, tag, resolvedImage.FullImageRef)
			i.image = resolvedImage.FullImageRef
			return
		}
		log.Debugf("No resolved image found for %s/apm-inject:%s, falling back to tag-based image", i.registry, tag)
		i.image = fmt.Sprintf("%s/apm-inject:%s", i.registry, tag)
	}
}

func injectorDebug(boolean string) injectorOption {
	var debug bool
	var err error
	if boolean != "" {
		debug, err = strconv.ParseBool(boolean)
		if err != nil {
			log.Errorf("parse admission.datadoghq.com/apm-inject.debug: %s", err)
		}
	}
	return func(i *injector) {
		i.debug = debug
	}
}

func injectorWithImageResolver(imageResolver ImageResolver) injectorOption {
	return func(i *injector) {
		i.imageResolver = imageResolver
	}
}

func newInjector(startTime time.Time, registry string, opts ...injectorOption) *injector {
	i := &injector{
		registry:   registry,
		injectTime: startTime,
	}

	for _, opt := range opts {
		opt(i)
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

var debugEnvVars = []envVar{
	{
		key:     "DD_APM_INSTRUMENTATION_DEBUG",
		valFunc: trueValFunc(),
	},
	{
		key:     "DD_TRACE_STARTUP_LOGS",
		valFunc: trueValFunc(),
	},
	{
		key:     "DD_TRACE_DEBUG",
		valFunc: trueValFunc(),
	},
}

func (i *injector) debugEnvVars() []envVar {
	if i.debug {
		return debugEnvVars
	}
	return nil
}
