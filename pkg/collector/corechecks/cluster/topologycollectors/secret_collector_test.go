// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package topologycollectors

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestSecretCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}

	cmc := NewSecretCollector(componentChannel, NewTestCommonClusterCollector(MockSecretAPICollectorClient{}))
	expectedCollectorName := "Secret Collector"
	RunCollectorTest(t, cmc, expectedCollectorName)

	type test struct {
		testCase string
		expected *topology.Component
	}

	for _, tc := range []struct {
		testCase string
		expected *topology.Component
	}{
		{
			testCase: "Test Secret 1 - Complete",
			expected: &topology.Component{
				ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:secret/test-secret-1",
				Type:       topology.Type{Name: "secret"},
				Data: topology.Data{
					"name":              "test-secret-1",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace"},
					"uid":               types.UID("test-secret-1"),
					"data":              "c20ca49dcb76feaaa1c14a2725263bf2290d0e5f3dc98d208b249f080fa64b45",
					"identifiers":       []string{"urn:kubernetes:/test-cluster-name:test-namespace:secret/test-secret-1"},
				},
			},
		},
		{
			testCase: "Test Secret 2 - Without Data",
			expected: &topology.Component{
				ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:secret/test-secret-2",
				Type:       topology.Type{Name: "secret"},
				Data: topology.Data{
					"name":              "test-secret-2",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace"},
					"uid":               types.UID("test-secret-2"),
					"data":              "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // Empty data is represented as a hash to obscure it
					"identifiers":       []string{"urn:kubernetes:/test-cluster-name:test-namespace:secret/test-secret-2"},
				},
			},
		},
		{
			testCase: "Test Secret 3 - Minimal",
			expected: &topology.Component{
				ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:secret/test-secret-3",
				Type:       topology.Type{Name: "secret"},
				Data: topology.Data{
					"name":              "test-secret-3",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"cluster-name": "test-cluster-name", "namespace": "test-namespace"},
					"uid":               types.UID("test-secret-3"),
					"data":              "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // Empty data is represented as a hash to obscure it
					"identifiers":       []string{"urn:kubernetes:/test-cluster-name:test-namespace:secret/test-secret-3"},
				},
			},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			component := <-componentChannel
			assert.EqualValues(t, tc.expected, component)
		})
	}

}

type MockSecretAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockSecretAPICollectorClient) GetSecrets() ([]coreV1.Secret, error) {
	secrets := make([]coreV1.Secret, 0)
	for i := 1; i <= 3; i++ {

		secret := coreV1.Secret{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-secret-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				UID:               types.UID(fmt.Sprintf("test-secret-%d", i)),
				GenerateName:      "",
			},
		}

		if i == 1 {
			secret.Data = map[string][]byte{
				"key1": asBase64("value1"),
				"key2": asBase64("longersecretvalue2"),
			}
		}

		if i != 3 {
			secret.Labels = map[string]string{
				"test": "label",
			}
		}

		secrets = append(secrets, secret)
	}

	return secrets, nil
}

func asBase64(s string) []byte {
	return []byte(base64.StdEncoding.EncodeToString([]byte(s)))
}
