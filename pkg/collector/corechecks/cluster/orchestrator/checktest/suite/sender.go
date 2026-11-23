// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/stretchr/testify/mock"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	orchestratorcheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
)

// Sent is one resource of a payload that the check sent.
type Sent struct {
	Metadata   bool
	Envelope   string
	UID        string
	Terminated bool
	// Content is the manifest content, or the YAML of the metadata model.
	Content []byte
	// MessageTags are the tags of the payload.
	MessageTags []string
	// Model is the whole decoded resource: the manifest, or the structured metadata model.
	Model any
}

// sendRecorder records the resources of the payloads that the check sends, from any goroutine.
type sendRecorder struct {
	sender *mocksender.MockSender
	mu     sync.Mutex
	sent   []Sent
}

func newSendRecorder(chk *orchestratorcheck.OrchestratorCheck, senderManager sender.SenderManager) *sendRecorder {
	r := &sendRecorder{sender: mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)}
	r.sender.On("OrchestratorMetadata", mock.Anything, mock.Anything, mock.Anything).Return().Run(func(args mock.Arguments) {
		r.record(metadataResources(args.Get(0).([]model.MessageBody)))
	})
	r.sender.On("OrchestratorManifest", mock.Anything, mock.Anything).Return().Run(func(args mock.Arguments) {
		r.record(manifestResources(args.Get(0).([]model.MessageBody)))
	})
	return r
}

func (r *sendRecorder) record(resources []Sent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, resources...)
}

// take returns the resources sent since the last call.
func (r *sendRecorder) take() []Sent {
	r.mu.Lock()
	defer r.mu.Unlock()
	sent := r.sent
	r.sent = nil
	return sent
}

func manifestResources(messages []model.MessageBody) []Sent {
	var resources []Sent
	for _, message := range messages {
		var envelope string
		var manifests *model.CollectorManifest
		switch m := message.(type) {
		case *model.CollectorManifest:
			envelope, manifests = "CollectorManifest", m
		case *model.CollectorManifestCR:
			envelope, manifests = "CollectorManifestCR", m.Manifest
		case *model.CollectorManifestCRD:
			envelope, manifests = "CollectorManifestCRD", m.Manifest
		default:
			envelope, manifests = fmt.Sprintf("%T", message), &model.CollectorManifest{}
		}
		for _, manifest := range manifests.Manifests {
			resources = append(resources, Sent{Envelope: envelope, UID: manifest.Uid,
				Terminated: manifest.IsTerminated, Content: manifest.Content, MessageTags: manifests.Tags, Model: manifest})
		}
	}
	return resources
}

// metadataResources returns the resources of structured metadata payloads. Each payload type holds a list of models
// with object metadata, except the cluster.
func metadataResources(messages []model.MessageBody) []Sent {
	var resources []Sent
	for _, message := range messages {
		if cluster, ok := message.(*model.CollectorCluster); ok {
			resources = append(resources, Sent{Metadata: true, Envelope: "CollectorCluster", UID: cluster.ClusterId, MessageTags: cluster.Tags, Model: cluster.Cluster})
			continue
		}
		payload := reflect.ValueOf(message).Elem()
		var messageTags []string
		if tagsField := payload.FieldByName("Tags"); tagsField.IsValid() {
			messageTags, _ = tagsField.Interface().([]string)
		}
		for i := range payload.NumField() {
			field := payload.Field(i)
			if field.Kind() != reflect.Slice || field.Type().Elem().Kind() != reflect.Pointer || field.Type().Elem().Elem().Kind() != reflect.Struct {
				continue
			}
			for j := range field.Len() {
				item := field.Index(j).Elem()
				if resource, ok := metadataResource(item, fmt.Sprintf("%T", message), messageTags); ok {
					resources = append(resources, resource)
				}
			}
		}
	}
	return resources
}

// metadataResource returns the resource of a structured metadata model, if the model has object metadata.
func metadataResource(item reflect.Value, envelope string, messageTags []string) (Sent, bool) {
	metadataField := item.FieldByName("Metadata")
	if !metadataField.IsValid() {
		return Sent{}, false
	}
	objectMetadata, ok := metadataField.Interface().(*model.Metadata)
	if !ok || objectMetadata == nil {
		return Sent{}, false
	}
	var yaml []byte
	if yamlField := item.FieldByName("Yaml"); yamlField.IsValid() {
		yaml = yamlField.Bytes()
	}
	return Sent{Metadata: true, Envelope: envelope, UID: objectMetadata.Uid, Terminated: objectMetadata.DeletionTimestamp != 0,
		Content: yaml, MessageTags: messageTags, Model: item.Addr().Interface()}, true
}
