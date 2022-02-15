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
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
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
	}

	var secrets []runtime.Object
	for _, rel := range releases {
		secret, err := secretForRelease(&rel)
		assert.NoError(t, err)
		secrets = append(secrets, secret)
	}

	// Add a secret not managed by Helm to verify that it doesn't cause issues.
	secrets = append(secrets, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "some_secret",
			Labels: map[string]string{"owner": "not-helm"},
		},
	})

	// Set up mocked k8s client and informer
	k8sClient := fake.NewSimpleClientset(secrets...)
	sharedK8sInformerFactory := informers.NewSharedInformerFactory(k8sClient, time.Minute)
	secretsInformer := sharedK8sInformerFactory.Core().V1().Secrets().Informer()
	go secretsInformer.Run(make(chan struct{}))
	err := apiserver.SyncInformers(
		map[apiserver.InformerName]cache.SharedInformer{"helm": secretsInformer},
		10*time.Second,
	)
	assert.NoError(t, err)

	check := &HelmCheck{
		CheckBase:         core.NewCheckBase(checkName),
		runLeaderElection: false,
		secretLister:      sharedK8sInformerFactory.Core().V1().Secrets().Lister(),
	}

	mockedSender := mocksender.NewMockSender(checkName)
	mockedSender.SetupAcceptAll()

	err = check.Run()
	assert.NoError(t, err)

	mockedSender.AssertMetric(t, "Gauge", "helm.release", 1, "", []string{
		"helm_release:my_datadog",
		"helm_chart_name:datadog",
		"helm_namespace:default",
		"helm_revision:1",
		"helm_status:deployed",
		"helm_chart_version:2.30.5",
		"helm_app_version:7",
	})

	mockedSender.AssertMetric(t, "Gauge", "helm.release", 1, "", []string{
		"helm_release:my_app",
		"helm_chart_name:some_app",
		"helm_namespace:app",
		"helm_revision:2",
		"helm_status:deployed",
		"helm_chart_version:1.1.0",
		"helm_app_version:1",
	})
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
