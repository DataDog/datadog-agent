//go:build kubeapiserver
// +build kubeapiserver

/*
Copyright 2017 The Kubernetes Authors.
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

// Taken from https://github.com/kubernetes/client-go/blob/v0.27.0/tools/leaderelection/resourcelock/configmaplock.go
// It was added because k8s.io/client-go/tools/leaderelection does not support ConfigMapLock anymore since v0.24 but
// it is needed to run leaderlection on kube versions <= 1.14, which do not support LeaseLocks
package resourcelock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"
)

const ConfigMapsResourceLock = "configmaps"

type ConfigMapLock struct {
	// ConfigMapMeta should contain a Name and a Namespace of a
	// ConfigMapMeta object that the LeaderElector will attempt to lead.
	ConfigMapMeta metav1.ObjectMeta
	Client        corev1client.ConfigMapsGetter
	LockConfig    rl.ResourceLockConfig
	cm            *v1.ConfigMap
}

// Get returns the election record from a ConfigMap Annotation
func (cml *ConfigMapLock) Get(ctx context.Context) (*rl.LeaderElectionRecord, []byte, error) {
	var record rl.LeaderElectionRecord
	cm, err := cml.Client.ConfigMaps(cml.ConfigMapMeta.Namespace).Get(ctx, cml.ConfigMapMeta.Name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}
	cml.cm = cm
	if cml.cm.Annotations == nil {
		cml.cm.Annotations = make(map[string]string)
	}
	recordStr, found := cml.cm.Annotations[rl.LeaderElectionRecordAnnotationKey]
	recordBytes := []byte(recordStr)
	if found {
		if err := json.Unmarshal(recordBytes, &record); err != nil {
			return nil, nil, err
		}
	}
	return &record, recordBytes, nil
}

// Create attempts to create a rl.LeaderElectionRecord annotation
func (cml *ConfigMapLock) Create(ctx context.Context, ler rl.LeaderElectionRecord) error {
	recordBytes, err := json.Marshal(ler)
	if err != nil {
		return err
	}
	cml.cm, err = cml.Client.ConfigMaps(cml.ConfigMapMeta.Namespace).Create(ctx, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cml.ConfigMapMeta.Name,
			Namespace: cml.ConfigMapMeta.Namespace,
			Annotations: map[string]string{
				rl.LeaderElectionRecordAnnotationKey: string(recordBytes),
			},
		},
	}, metav1.CreateOptions{})
	return err
}

// Update will update an existing annotation on a given resource.
func (cml *ConfigMapLock) Update(ctx context.Context, ler rl.LeaderElectionRecord) error {
	if cml.cm == nil {
		return errors.New("configmap not initialized, call get or create first")
	}
	recordBytes, err := json.Marshal(ler)
	if err != nil {
		return err
	}
	if cml.cm.Annotations == nil {
		cml.cm.Annotations = make(map[string]string)
	}
	cml.cm.Annotations[rl.LeaderElectionRecordAnnotationKey] = string(recordBytes)
	cm, err := cml.Client.ConfigMaps(cml.ConfigMapMeta.Namespace).Update(ctx, cml.cm, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	cml.cm = cm
	return nil
}

// RecordEvent in leader election while adding meta-data
func (cml *ConfigMapLock) RecordEvent(s string) {
	if cml.LockConfig.EventRecorder == nil {
		return
	}
	events := fmt.Sprintf("%v %v", cml.LockConfig.Identity, s)
	subject := &v1.ConfigMap{ObjectMeta: cml.cm.ObjectMeta}
	// Populate the type meta, so we don't have to get it from the schema
	subject.Kind = "ConfigMap"
	subject.APIVersion = v1.SchemeGroupVersion.String()
	cml.LockConfig.EventRecorder.Eventf(subject, v1.EventTypeNormal, "LeaderElection", events)
}

// Describe is used to convert details on current resource lock
// into a string
func (cml *ConfigMapLock) Describe() string {
	return fmt.Sprintf("%v/%v", cml.ConfigMapMeta.Namespace, cml.ConfigMapMeta.Name)
}

// Identity returns the Identity of the lock
func (cml *ConfigMapLock) Identity() string {
	return cml.LockConfig.Identity
}
