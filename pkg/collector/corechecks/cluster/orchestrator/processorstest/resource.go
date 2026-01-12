// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

//nolint:revive
package processorstest

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Resource is a test resource for processors.
type Resource struct {
	ObjectMeta                       ObjectMeta `json:"object_meta"`
	Property                         string     `json:"property"`
	PropertyToSetAfterMarshalling    string     `json:"property_to_set_after_marshalling"`
	PropertyToSetBeforeMarshalling   string     `json:"property_to_set_before_marshalling"`
	PropertyToSetBeforeCacheCheck    string     `json:"property_to_set_before_cache_check"`
	PropertyToScrubBeforeExtraction  string     `json:"property_to_scrub_before_extraction"`
	PropertyToScrubBeforeMarshalling string     `json:"property_to_scrub_before_marshalling"`
	ResourceVersion                  string     `json:"resource_version"`
	ResourceUID                      string     `json:"resource_uid"`
}

// ObjectMeta is a test object meta for a test resource.
type ObjectMeta struct {
	DeletionTimestamp *metav1.Time `json:"deletion_timestamp"`
}

// DeepCopy creates a deep copy of the resource.
func (r *Resource) DeepCopy() *Resource {
	originalResource := MustMarshalJSON(r)
	resourceCopy := MustUnmarshalJSON[Resource](originalResource)
	return &resourceCopy
}

// NewResource creates a new resource to be used as input for processor tests.
func NewResource() *Resource {
	return &Resource{
		Property:                         "value",
		PropertyToScrubBeforeExtraction:  "secret1",
		PropertyToScrubBeforeMarshalling: "secret2",
		ResourceVersion:                  "1",
		ResourceUID:                      "uid",
	}
}

// NewExpectedResourceMetadata returns the expected state of the resource created with [NewResource] after successful metadata processing.
func NewExpectedResourceMetadata() *Resource {
	return &Resource{
		Property:                         "value",
		PropertyToScrubBeforeExtraction:  "scrubbed-before-extraction",
		PropertyToScrubBeforeMarshalling: "secret2",
		ResourceVersion:                  "1",
		ResourceUID:                      "uid",
		PropertyToSetBeforeMarshalling:   "value-before-marshalling",
		PropertyToSetBeforeCacheCheck:    "value-before-cache-check",
	}
}

// NewExpectedResourceManifest returns the expected state of the resource created with [NewResource] after successful manifest processing.
func NewExpectedResourceManifest() *Resource {
	return &Resource{
		Property:                         "value",
		PropertyToScrubBeforeExtraction:  "scrubbed-before-extraction",
		PropertyToScrubBeforeMarshalling: "scrubbed-before-marshalling",
		ResourceVersion:                  "1",
		ResourceUID:                      "uid",
		PropertyToSetBeforeMarshalling:   "value-before-marshalling",
		PropertyToSetBeforeCacheCheck:    "value-before-cache-check",
	}
}

// MustMarshalJSON marshals a resource to JSON and panics if the marshalling fails.
// Only to be used in tests.
func MustMarshalJSON(resource any) []byte {
	json, err := json.Marshal(resource)
	if err != nil {
		panic(err)
	}
	return json
}

// MustUnmarshalJSON unmarshals a resource from JSON and panics if the unmarshalling fails.
// Only to be used in tests.
func MustUnmarshalJSON[T any](data []byte) T {
	var resource T

	if err := json.Unmarshal(data, &resource); err != nil {
		panic(err)
	}

	return resource
}
