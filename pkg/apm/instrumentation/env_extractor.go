// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver || kubelet

package instrumentation

import (
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ExtractedTags are USTs that we could have been setting on the pod
// with the instrumentation webhook via workload targeting.
type ExtractedTags struct {
	Service string
	Env     string
	Version string
}

// AsMap produces a map of UST to value for the ones that
// are set on ExtractedTags.
func (e *ExtractedTags) AsMap() map[string]string {
	if e == nil {
		return nil
	}

	usts := [3]string{tags.Service, tags.Env, tags.Version}
	vals := [3]string{e.Service, e.Env, e.Version}

	var out map[string]string
	for idx, val := range vals {
		if val != "" {
			if out == nil {
				out = map[string]string{}
			}
			out[usts[idx]] = val
		}
	}

	return out
}

func writeService(e *ExtractedTags, name string) {
	e.Service = name
}

func writeEnv(e *ExtractedTags, name string) {
	e.Env = name
}

func writeVersion(e *ExtractedTags, name string) {
	e.Version = name
}

func parseFromTracerConfig(tc TracerConfig, m metav1.ObjectMeta) (string, func(*ExtractedTags, string)) {
	var do func(*ExtractedTags, string)
	switch tc.Name {
	case kubernetes.ServiceTagEnvVar:
		log.Debug("setting writeService")
		do = writeService
	case kubernetes.VersionTagEnvVar:
		log.Debug("setting writeVersion")
		do = writeVersion
	case kubernetes.EnvTagEnvVar:
		log.Debug("setting writeEnv")
		do = writeEnv
	default:
		return "", nil
	}

	value, extracted, err := extractSingleValueFromPodMeta(m, tc)
	if err != nil {
		log.Warnf("error parsing value workload data from pod metadata for env %s: %s", tc.Name, err)
		return "", nil
	}
	if !extracted {
		return "", nil
	}

	return value, do
}

// ExtractTagsFromPodMeta extracts ExtractedTags from a given pod metadata.
//
// This checks for the [[AppliedTargetAnnotation]] and checks for
// any trace configurations that can be corresponding to USTs.
func ExtractTagsFromPodMeta(in metav1.ObjectMeta) (*ExtractedTags, error) {
	tJSON, ok := in.Annotations[AppliedTargetAnnotation]
	if !ok {
		return nil, nil
	}

	var t Target
	if err := json.NewDecoder(strings.NewReader(tJSON)).Decode(&t); err != nil {
		return nil, fmt.Errorf("error parsing instrumentation target JSON: %s", err)
	}

	var out *ExtractedTags
	for _, tc := range t.TracerConfigs {
		if value, writer := parseFromTracerConfig(tc, in); writer != nil && value != "" {
			if out == nil {
				out = new(ExtractedTags)
			}
			writer(out, value)
		}
	}

	return out, nil
}

// extractSingleValueFromPodMeta is largely copied from kubernetes source but uses the tracer
// config to provide the target.
//
// ref: https://github.com/kubernetes/kubernetes/blob/6da56bd4b782658a4060f65c24df5830ec01c6c1/pkg/fieldpath/fieldpath.go#L53-L120
func extractSingleValueFromPodMeta(meta metav1.ObjectMeta, c TracerConfig) (string, bool, error) {
	if c.ValueFrom == nil {
		return c.Value, c.Value != "", nil
	}

	if c.ValueFrom.FieldRef == nil {
		return "", false, nil
	}

	fieldPath := c.ValueFrom.FieldRef.FieldPath
	if path, subscript, ok := splitMaybeSubscriptedPath(fieldPath); ok {
		switch path {
		case "metadata.annotations":
			value, present := meta.Annotations[subscript]
			return value, present, nil
		case "metadata.labels":
			value, present := meta.Labels[subscript]
			return value, present, nil
		default:
			return "", false, fmt.Errorf("invalid fieldPath with subscript %s", fieldPath)
		}
	}

	switch fieldPath {
	case "metadata.name":
		return meta.Name, true, nil
	case "metadata.namespace":
		return meta.Namespace, true, nil
	case "metadata.uid":
		return string(meta.GetUID()), true, nil
	}

	return "", false, fmt.Errorf("unsupported access of fieldPath %s", fieldPath)
}

// splitMaybeSubscriptedPath checks whether the specified fieldPath is
// subscripted, and
//   - if yes, this function splits the fieldPath into path and subscript, and
//     returns (path, subscript, true).
//   - if no, this function returns (fieldPath, "", false).
//
// Example inputs and outputs:
//
//	"metadata.annotations['myKey']" --> ("metadata.annotations", "myKey", true)
//	"metadata.annotations['a[b]c']" --> ("metadata.annotations", "a[b]c", true)
//	"metadata.labels['']"           --> ("metadata.labels", "", true)
//	"metadata.labels"               --> ("metadata.labels", "", false)
func splitMaybeSubscriptedPath(fieldPath string) (string, string, bool) {
	if !strings.HasSuffix(fieldPath, "']") {
		return fieldPath, "", false
	}
	s := strings.TrimSuffix(fieldPath, "']")
	parts := strings.SplitN(s, "['", 2)
	if len(parts) < 2 {
		return fieldPath, "", false
	}
	if len(parts[0]) == 0 {
		return fieldPath, "", false
	}
	return parts[0], parts[1], true
}
