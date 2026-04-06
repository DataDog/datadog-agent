// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/mocktracer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestReplace_CreatesSpan(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	workloadmetaComponent := mockedWorkloadmeta(t)

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}
	parser, err := kubernetesresourceparsers.NewMetadataParser(gvr, nil)
	require.NoError(t, err)

	store := &reflectorStore{
		wlmetaStore: workloadmetaComponent,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      parser,
	}

	obj1 := &metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ns1",
			UID:  "uid-1",
		},
	}
	obj2 := &metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ns2",
			UID:  "uid-2",
		},
	}

	err = store.Replace([]interface{}{obj1, obj2}, "")
	require.NoError(t, err)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)

	span := spans[0]
	assert.Equal(t, "workloadmeta.reflector_store.replace", span.OperationName())
	assert.Equal(t, 2, span.Tag("list_size"))
	assert.Equal(t, 2, span.Tag("events_generated"))
	assert.Nil(t, span.Tag("error.message"))
}

func TestReplace_SpanRecordsEventsForDeletedEntities(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	workloadmetaComponent := mockedWorkloadmeta(t)

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}
	parser, err := kubernetesresourceparsers.NewMetadataParser(gvr, nil)
	require.NoError(t, err)

	store := &reflectorStore{
		wlmetaStore: workloadmetaComponent,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      parser,
	}

	// First Replace to seed the store with 2 objects
	obj1 := &metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ns1",
			UID:  "uid-1",
		},
	}
	obj2 := &metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ns2",
			UID:  "uid-2",
		},
	}
	err = store.Replace([]interface{}{obj1, obj2}, "")
	require.NoError(t, err)

	mt.Reset()

	// Second Replace with only one object should produce 1 set + 1 unset = 2 events
	err = store.Replace([]interface{}{obj1}, "")
	require.NoError(t, err)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)

	span := spans[0]
	assert.Equal(t, "workloadmeta.reflector_store.replace", span.OperationName())
	assert.Equal(t, 1, span.Tag("list_size"))
	assert.Equal(t, 2, span.Tag("events_generated"))
}

func TestReplace_EmptyList_CreatesSpan(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	workloadmetaComponent := mockedWorkloadmeta(t)

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}
	parser, err := kubernetesresourceparsers.NewMetadataParser(gvr, nil)
	require.NoError(t, err)

	store := &reflectorStore{
		wlmetaStore: workloadmetaComponent,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      parser,
	}

	err = store.Replace(nil, "")
	require.NoError(t, err)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)

	span := spans[0]
	assert.Equal(t, "workloadmeta.reflector_store.replace", span.OperationName())
	assert.Equal(t, 0, span.Tag("list_size"))
	assert.Equal(t, 0, span.Tag("events_generated"))
}
