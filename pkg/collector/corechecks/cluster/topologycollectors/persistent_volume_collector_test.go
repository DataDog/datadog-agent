// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package topologycollectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"
)

var volumeLocation coreV1.HostPathType
var gcePersistentDisk coreV1.GCEPersistentDiskVolumeSource
var awsElasticBlockStore coreV1.AWSElasticBlockStoreVolumeSource
var hostPath coreV1.HostPathVolumeSource

func TestPersistentVolumeCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}
	volumeLocation = coreV1.HostPathFileOrCreate
	gcePersistentDisk = coreV1.GCEPersistentDiskVolumeSource{
		PDName: "name-of-the-gce-persistent-disk",
	}
	awsElasticBlockStore = coreV1.AWSElasticBlockStoreVolumeSource{
		VolumeID: "id-of-the-aws-block-store",
	}
	hostPath = coreV1.HostPathVolumeSource{
		Path: "some/path/to/the/volume",
		Type: &volumeLocation,
	}

	cmc := NewPersistentVolumeCollector(componentChannel, NewTestCommonClusterCollector(MockPersistentVolumeAPICollectorClient{}))
	expectedCollectorName := "Persistent Volume Collector"
	RunCollectorTest(t, cmc, expectedCollectorName)

	for _, tc := range []struct {
		testCase string
		expected *topology.Component
	}{
		{
			testCase: "Test Persistent Volume 1 - AWS Elastic Block Store",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:persistent-volume:test-persistent-volume-1",
				Type:       topology.Type{Name: "persistent-volume"},
				Data: topology.Data{
					"name":              "test-persistent-volume-1",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-persistent-volume-1"),
					"identifiers":       []string{},
					"status":            coreV1.VolumeAvailable,
					"statusMessage":     "Volume is available for use",
					"storageClassName":  "Storage-Class-Name",
					"source": coreV1.PersistentVolumeSource{
						AWSElasticBlockStore: &awsElasticBlockStore,
					},
				},
			},
		},
		{
			testCase: "Test Persistent Volume 2 - GCE Persistent Disk",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:persistent-volume:test-persistent-volume-2",
				Type:       topology.Type{Name: "persistent-volume"},
				Data: topology.Data{
					"name":              "test-persistent-volume-2",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-persistent-volume-2"),
					"identifiers":       []string{},
					"status":            coreV1.VolumeAvailable,
					"statusMessage":     "Volume is available for use",
					"storageClassName":  "Storage-Class-Name",
					"source": coreV1.PersistentVolumeSource{
						GCEPersistentDisk: &gcePersistentDisk,
					},
				},
			},
		},
		{
			testCase: "Test Persistent Volume 3 - Host Path + Kind + Generate Name",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:persistent-volume:test-persistent-volume-3",
				Type:       topology.Type{Name: "persistent-volume"},
				Data: topology.Data{
					"name":              "test-persistent-volume-3",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-persistent-volume-3"),
					"identifiers":       []string{},
					"kind":              "some-specified-kind",
					"generateName":      "some-specified-generation",
					"status":            coreV1.VolumeAvailable,
					"statusMessage":     "Volume is available for use",
					"storageClassName":  "Storage-Class-Name",
					"source": coreV1.PersistentVolumeSource{
						HostPath: &hostPath,
					},
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

type MockPersistentVolumeAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockPersistentVolumeAPICollectorClient) GetPersistentVolumes() ([]coreV1.PersistentVolume, error) {
	persistentVolumes := make([]coreV1.PersistentVolume, 0)
	for i := 1; i <= 3; i++ {
		persistentVolume := coreV1.PersistentVolume{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-persistent-volume-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-persistent-volume-%d", i)),
				GenerateName: "",
			},
			Spec: coreV1.PersistentVolumeSpec{
				StorageClassName: "Storage-Class-Name",
			},
			Status: coreV1.PersistentVolumeStatus{
				Phase:   coreV1.VolumeAvailable,
				Message: "Volume is available for use",
			},
		}

		if i == 1 {
			persistentVolume.Spec.PersistentVolumeSource = coreV1.PersistentVolumeSource{
				AWSElasticBlockStore: &awsElasticBlockStore,
			}
		}

		if i == 2 {
			persistentVolume.Spec.PersistentVolumeSource = coreV1.PersistentVolumeSource{
				GCEPersistentDisk: &gcePersistentDisk,
			}
		}

		if i == 3 {
			persistentVolume.Spec.PersistentVolumeSource = coreV1.PersistentVolumeSource{
				HostPath: &hostPath,
			}
			persistentVolume.TypeMeta.Kind = "some-specified-kind"
			persistentVolume.ObjectMeta.GenerateName = "some-specified-generation"
		}

		persistentVolumes = append(persistentVolumes, persistentVolume)
	}

	return persistentVolumes, nil
}
