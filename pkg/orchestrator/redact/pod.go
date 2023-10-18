// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package redact

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	v1 "k8s.io/api/core/v1"
)

const (
	redactedValue = "********"
	replacedValue = "-"
)

// ScrubPodTemplateSpec scrubs a pod template.
func ScrubPodTemplateSpec(template *v1.PodTemplateSpec, scrubber *DataScrubber) {
	ScrubAnnotations(template.Annotations, scrubber)

	for c := 0; c < len(template.Spec.InitContainers); c++ {
		ScrubContainer(&template.Spec.InitContainers[c], scrubber)
	}
	for c := 0; c < len(template.Spec.Containers); c++ {
		ScrubContainer(&template.Spec.Containers[c], scrubber)
	}
}

// ScrubPod scrubs a pod.
func ScrubPod(p *v1.Pod, scrubber *DataScrubber) {
	ScrubAnnotations(p.Annotations, scrubber)

	for c := 0; c < len(p.Spec.InitContainers); c++ {
		ScrubContainer(&p.Spec.InitContainers[c], scrubber)
	}
	for c := 0; c < len(p.Spec.Containers); c++ {
		ScrubContainer(&p.Spec.Containers[c], scrubber)
	}
}

// ScrubAnnotations scrubs sensitive information from pod annotations.
func ScrubAnnotations(annotations map[string]string, scrubber *DataScrubber) {
	for k, v := range annotations {
		annotations[k] = scrubber.ScrubAnnotationValue(v)
	}
}

// ScrubContainer scrubs sensitive information in the command line & env vars
func ScrubContainer(c *v1.Container, scrubber *DataScrubber) {
	// scrub env vars
	for e := 0; e < len(c.Env); e++ {
		if scrubber.ContainsSensitiveWord(c.Env[e].Name) {
			c.Env[e].Value = redactedValue
		}
	}

	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Failed to parse cmd from pod, obscuring whole command")
			// we still want to obscure to be safe
			c.Command = []string{redactedValue}
		}
	}()

	// scrub args and commands
	merged := append(c.Command, c.Args...)
	words := 0
	for _, cmd := range c.Command {
		words += len(strings.Split(cmd, " "))
	}

	scrubbedMergedCommand, changed := scrubber.ScrubSimpleCommand(merged) // return value is split if has been changed
	if !changed {
		return // no change has happened, no need to go further down the line
	}

	// if part of the merged command got scrubbed the updated value will be split, even for e.g. c.Args only if the c.Command got scrubbed
	if len(c.Command) > 0 {
		c.Command = scrubbedMergedCommand[:words]
	}
	if len(c.Args) > 0 {
		c.Args = scrubbedMergedCommand[words:]
	}
}

// RemoveLastAppliedConfigurationAnnotation redacts the whole "kubectl.kubernetes.io/last-applied-configuration" annotation. As it may contain duplicate information and secrets.
func RemoveLastAppliedConfigurationAnnotation(annotations map[string]string) {
	if _, found := annotations[v1.LastAppliedConfigAnnotation]; found {
		annotations[v1.LastAppliedConfigAnnotation] = replacedValue
	}
}
