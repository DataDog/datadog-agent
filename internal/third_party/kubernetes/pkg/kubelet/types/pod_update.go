/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package types

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
)

const (
	// ConfigSourceAnnotationKey represents the annotation key for pod config source.
	ConfigSourceAnnotationKey = "kubernetes.io/config.source"

	// These constants identify the sources of pods

	// FileSource represents the file config source.
	// Updates from a file
	FileSource = "file"
	// HTTPSource represents the http config source.
	// Updates from querying a web page
	HTTPSource = "http"
	// ApiserverSource represents the api-server config source.
	// Updates from Kubernetes API Server
	ApiserverSource = "api"
	// AllSource represents all config sources.
	// Updates from all sources
	AllSource = "*"
)

// GetPodSource returns the source of the pod based on the annotation.
func GetPodSource(pod *v1.Pod) (string, error) {
	if pod.Annotations != nil {
		if source, ok := pod.Annotations[ConfigSourceAnnotationKey]; ok {
			return source, nil
		}
	}
	return "", fmt.Errorf("cannot get source of pod %q", pod.UID)
}

// IsStaticPod returns true if the pod is a static pod.
func IsStaticPod(pod *v1.Pod) bool {
	source, err := GetPodSource(pod)
	return err == nil && source != ApiserverSource
}
