// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestScrubPodTemplateResource(t *testing.T) {
	template := &v1.PodTemplate{ObjectMeta: sensitiveObjectMeta(), Template: sensitivePodTemplateSpec()}
	ScrubPodTemplate(template, NewDefaultDataScrubber())
	assertScrubbedObjectMeta(t, template.ObjectMeta)
	assertScrubbedPodTemplateSpec(t, template.Template)
}

func TestScrubReplicationController(t *testing.T) {
	podTemplate := sensitivePodTemplateSpec()
	controller := &v1.ReplicationController{ObjectMeta: sensitiveObjectMeta(), Spec: v1.ReplicationControllerSpec{Template: &podTemplate}}
	ScrubReplicationController(controller, NewDefaultDataScrubber())
	assertScrubbedObjectMeta(t, controller.ObjectMeta)
	assertScrubbedPodTemplateSpec(t, *controller.Spec.Template)

	withoutTemplate := &v1.ReplicationController{ObjectMeta: sensitiveObjectMeta()}
	ScrubReplicationController(withoutTemplate, NewDefaultDataScrubber())
	assertScrubbedObjectMeta(t, withoutTemplate.ObjectMeta)
	assert.Nil(t, withoutTemplate.Spec.Template)
}

func TestScrubConfigMap(t *testing.T) {
	configMap := &v1.ConfigMap{
		ObjectMeta: sensitiveObjectMeta(),
		Data:       map[string]string{"password": "sensitive-value", "port": "8080"},
		BinaryData: map[string][]byte{"api_key": []byte("sensitive-value"), "public": []byte("hello")},
	}
	ScrubConfigMap(configMap, NewDefaultDataScrubber())
	assertScrubbedObjectMeta(t, configMap.ObjectMeta)
	assert.Equal(t, map[string]string{"password": redactedSecret, "port": "8080"}, configMap.Data)
	assert.Equal(t, map[string][]byte{"api_key": []byte(redactedSecret), "public": []byte("hello")}, configMap.BinaryData)
}

func TestScrubMetadataOnlyResource(t *testing.T) {
	service := &v1.Service{ObjectMeta: sensitiveObjectMeta(), Spec: v1.ServiceSpec{ClusterIP: "10.0.0.1"}}
	ScrubService(service, NewDefaultDataScrubber())
	assertScrubbedObjectMeta(t, service.ObjectMeta)
	assert.Equal(t, "10.0.0.1", service.Spec.ClusterIP)
}
