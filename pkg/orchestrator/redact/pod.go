// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package redact

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	v1 "k8s.io/api/core/v1"
)

const (
	redactedValue = "********"
)

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

// TODO: we need to access the env for this
// Alternative idea: unmarshal to pod spec
// lets try the log scrubber
func ScrubAnnotations(o *metav1.ObjectMeta, scrubber *DataScrubber) {
	annotations := o.Annotations["kubectl.kubernetes.io/last-applied-configuration"]
	msgScrubbed, err := log.CredentialsCleanerBytes([]byte(annotations))
	if err == nil {
		log.Errorf("%v", string(msgScrubbed))
	} else {
		log.Errorf("failure: %v", err)
	}
}
