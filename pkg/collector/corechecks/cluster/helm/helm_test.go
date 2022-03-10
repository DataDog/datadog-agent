// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package helm

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func TestRun(t *testing.T) {
	releases := []release{
		{
			Name: "my_datadog",
			Info: &info{
				Status: "deployed",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "datadog",
					Version:    "2.30.5",
					AppVersion: "7",
				},
			},
			Version:   1,
			Namespace: "default",
		},
		{
			Name: "my_app",
			Info: &info{
				Status: "deployed",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "some_app",
					Version:    "1.1.0",
					AppVersion: "1",
				},
			},
			Version:   2,
			Namespace: "app",
		},
		{ // Release with a nil chart reference
			Name: "release_without_chart",
			Info: &info{
				Status: "deployed",
			},
			Chart:     nil,
			Version:   1,
			Namespace: "default",
		},
		{ // Release with a nil info reference
			Name: "release_without_info",
			Info: nil,
			Chart: &chart{
				Metadata: &metadata{
					Name:       "example_app",
					Version:    "2.0.0",
					AppVersion: "1",
				},
			},
			Version:   1,
			Namespace: "default",
		},
	}

	// Same order as "releases" array
	var secretsForReleases []*v1.Secret
	for _, rel := range releases {
		secret, err := secretForRelease(&rel)
		assert.NoError(t, err)
		secretsForReleases = append(secretsForReleases, secret)
	}

	// Same order as "releases" array
	var configmapsForReleases []*v1.ConfigMap
	for _, rel := range releases {
		configMap, err := configMapForRelease(&rel)
		assert.NoError(t, err)
		configmapsForReleases = append(configmapsForReleases, configMap)
	}

	// Same order as "releases" array
	expectedTagsForReleases := [][]string{
		{
			"helm_release:my_datadog",
			"helm_chart_name:datadog",
			"helm_namespace:default",
			"helm_revision:1",
			"helm_status:deployed",
			"helm_chart_version:2.30.5",
			"helm_app_version:7",
		},
		{
			"helm_release:my_app",
			"helm_chart_name:some_app",
			"helm_namespace:app",
			"helm_revision:2",
			"helm_status:deployed",
			"helm_chart_version:1.1.0",
			"helm_app_version:1",
		},
		{
			"helm_release:release_without_chart",
			"helm_namespace:default",
			"helm_revision:1",
			"helm_status:deployed",
		},
		{
			"helm_release:release_without_info",
			"helm_chart_name:example_app",
			"helm_namespace:default",
			"helm_revision:1",
			"helm_chart_version:2.0.0",
			"helm_app_version:1",
		},
	}

	tests := []struct {
		name         string
		secrets      []*v1.Secret
		configmaps   []*v1.ConfigMap
		expectedTags [][]string
	}{
		{
			name:         "using secrets",
			secrets:      secretsForReleases,
			expectedTags: expectedTagsForReleases,
		},
		{
			name:         "using configmaps",
			configmaps:   configmapsForReleases,
			expectedTags: expectedTagsForReleases,
		},
		{
			name:         "using secrets and configmaps",
			secrets:      []*v1.Secret{secretsForReleases[0]},
			configmaps:   configmapsForReleases[1:],
			expectedTags: expectedTagsForReleases,
		},
		{
			name: "no secrets or configmaps owned by Helm",
			secrets: []*v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "some_secret",
						Labels: map[string]string{"owner": "not-helm"},
					},
				},
			},
			configmaps: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "some_configmap",
						Labels: map[string]string{"owner": "not-helm"},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stopCh := make(chan struct{})
			defer close(stopCh)

			var kubeObjects []runtime.Object
			for _, secret := range test.secrets {
				kubeObjects = append(kubeObjects, secret)
			}
			for _, configMap := range test.configmaps {
				kubeObjects = append(kubeObjects, configMap)
			}

			check := factory().(*HelmCheck)
			check.runLeaderElection = false

			k8sClient := fake.NewSimpleClientset(kubeObjects...)
			sharedK8sInformerFactory := informers.NewSharedInformerFactory(k8sClient, time.Minute)
			err := check.setupInformers(sharedK8sInformerFactory)
			assert.NoError(t, err)

			mockedSender := mocksender.NewMockSender(checkName)
			mockedSender.SetupAcceptAll()

			err = check.Run()
			assert.NoError(t, err)

			for _, tags := range test.expectedTags {
				mockedSender.AssertMetric(t, "Gauge", "helm.release", 1, "", tags)
			}
		})
	}
}

// secretForRelease returns a Kubernetes secret that contains the info of the
// given Helm release.
func secretForRelease(rls *release) (*v1.Secret, error) {
	encodedRel, err := encodeRelease(rls)
	if err != nil {
		return nil, err
	}

	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			// The name is not important for this test. We only need to make
			// sure that there are no collisions.
			Name:   fmt.Sprintf("%s.%d", rls.Name, rls.Version),
			Labels: map[string]string{"owner": "helm"},
		},
		Data: map[string][]byte{"release": []byte(encodedRel)},
	}, nil
}

// configMapForRelease returns a configmap that contains the info of the given
// Helm release.
func configMapForRelease(rls *release) (*v1.ConfigMap, error) {
	encodedRel, err := encodeRelease(rls)
	if err != nil {
		return nil, err
	}

	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			// The name is not important for this test. We only need to make
			// sure that there are no collisions.
			Name:   fmt.Sprintf("%s.%d", rls.Name, rls.Version),
			Labels: map[string]string{"owner": "helm"},
		},
		Data: map[string]string{"release": encodedRel},
	}, nil
}

// Note: the encodeRelease function has been copied from the Helm library.
// It's private, so we can't reuse it.
// Ref: https://github.com/helm/helm/blob/v3.8.0/pkg/storage/driver/util.go#L35

// encodeRelease encodes a release returning a base64 encoded
// gzipped string representation, or error.
func encodeRelease(rls *release) (string, error) {
	b, err := json.Marshal(rls)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err = w.Write(b); err != nil {
		return "", err
	}
	w.Close()

	return b64.EncodeToString(buf.Bytes()), nil
}
