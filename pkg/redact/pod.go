// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package redact

import (
	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RemoveSensitiveAnnotationsAndLabels redacts sensitive annotations and labels like the whole
// "kubectl.kubernetes.io/last-applied-configuration" annotation value. As it
// may contain duplicate information and secrets.
func RemoveSensitiveAnnotationsAndLabels(annotations map[string]string, labels map[string]string) {
	for _, v := range GetSensitiveAnnotationsAndLabels() {
		if _, found := annotations[v]; found {
			annotations[v] = redactedAnnotationValue
		}
		if _, found := labels[v]; found {
			labels[v] = redactedAnnotationValue
		}
	}
}

// ScrubPodTemplateSpec scrubs a pod template.
func ScrubPodTemplateSpec(template *v1.PodTemplateSpec, scrubber *DataScrubber) {
	scrubAnnotations(template.Annotations, scrubber)

	for c := 0; c < len(template.Spec.InitContainers); c++ {
		scrubContainer(&template.Spec.InitContainers[c], scrubber)
	}
	for c := 0; c < len(template.Spec.Containers); c++ {
		scrubContainer(&template.Spec.Containers[c], scrubber)
	}
}

// ScrubPod scrubs a pod.
func ScrubPod(p *v1.Pod, scrubber *DataScrubber) {
	scrubAnnotations(p.Annotations, scrubber)

	for c := 0; c < len(p.Spec.InitContainers); c++ {
		scrubContainer(&p.Spec.InitContainers[c], scrubber)
	}
	for c := 0; c < len(p.Spec.Containers); c++ {
		scrubContainer(&p.Spec.Containers[c], scrubber)
	}
}

// scrubAnnotations scrubs sensitive information from pod annotations.
func scrubAnnotations(annotations map[string]string, scrubber *DataScrubber) {
	for k, v := range annotations {
		annotations[k] = scrubber.ScrubAnnotationValue(v)
	}
}

func scrubContainerProbe(probe *v1.Probe, scrubber *DataScrubber) {
	if probe == nil {
		return
	}

	if probe.HTTPGet != nil {
		for h := 0; h < len(probe.HTTPGet.HTTPHeaders); h++ {
			if scrubber.ContainsSensitiveWord(probe.HTTPGet.HTTPHeaders[h].Name) {
				probe.HTTPGet.HTTPHeaders[h].Value = redactedSecret
			}
		}
	}

	if probe.Exec != nil {
		probe.Exec.Command, _, _ = scrubber.ScrubSimpleCommand(probe.Exec.Command, nil)
	}
}

// scrubContainer scrubs sensitive information in the command line & env vars
func scrubContainer(c *v1.Container, scrubber *DataScrubber) {
	// scrub env vars
	for e := 0; e < len(c.Env); e++ {
		if scrubber.ContainsSensitiveWord(c.Env[e].Name) {
			// It's possible the env var is set using a ValueFrom field, in which case we don't want to scrub the value field
			if c.Env[e].Value != "" {
				c.Env[e].Value = redactedSecret
			}
		}
	}

	// scrub probes http headers
	scrubContainerProbe(c.LivenessProbe, scrubber)
	scrubContainerProbe(c.ReadinessProbe, scrubber)
	scrubContainerProbe(c.StartupProbe, scrubber)

	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Failed to parse cmd from pod, obscuring whole command, container: %s, error: %v", c.Name, r)
			// we still want to obscure to be safe
			c.Command = []string{redactedSecret}
		}
	}()

	scrubbedCommand, scrubbedArg, changed := scrubber.ScrubSimpleCommand(c.Command, c.Args) // return value is split if has been changed
	if !changed {
		return // no change has happened, no need to go further down the line
	}
	if len(c.Command) > 0 {
		c.Command = scrubbedCommand
	}
	if len(c.Args) > 0 {
		c.Args = scrubbedArg
	}
}
