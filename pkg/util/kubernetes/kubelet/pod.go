// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	json "github.com/json-iterator/go"
)

type creatorRef struct {
	Kind      string
	Reference PodOwner
}

// Owners returns pod owners, sourced either from:
// - the new `Owners` field, exposed by the kubelet since 1.6
// - the legacy `kubernetes.io/created-by` annotation, deprecated in 1.8
func (p *Pod) Owners() []PodOwner {
	// If we find the new field, return it
	owners := p.Metadata.Owners
	if len(owners) > 0 {
		return owners
	}

	// Else, try unserialising the legacy field
	content, found := p.Metadata.Annotations["kubernetes.io/created-by"]
	if !found {
		return nil
	}
	var ref creatorRef
	err := json.Unmarshal([]byte(content), &ref)

	// Error handling
	if err != nil {
		log.Debugf("Cannot parse created-by field for pod %q: %s", p.Metadata.Name, err)
		return nil
	}
	if ref.Kind != "SerializedReference" {
		log.Debugf("Cannot parse created-by field for pod %q: unknown kind %q", p.Metadata.Name, ref.Kind)
		return nil
	}

	owners = []PodOwner{ref.Reference}
	return owners
}
