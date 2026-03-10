// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package controllers

import (
	"testing"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestSubscribe(t *testing.T) {
	store := newTestStore()

	ch := store.Subscribe("node1")
	defer store.Unsubscribe("node1")

	bundle := apiserver.NewMetadataMapperBundle()
	bundle.Services.Set("default", "pod1", "svc1")
	store.set("node1", bundle)

	select {
	case <-ch:
		// expected
	case <-time.After(1 * time.Second):
		t.Fatal("was not notified")
	}

	select {
	case <-ch:
		t.Fatal("received unexpected notification")
	default:
	}
}

func TestSubscribe_WithDelete(t *testing.T) {
	store := newTestStore()

	bundle := apiserver.NewMetadataMapperBundle()
	bundle.Services.Set("default", "pod1", "svc1")
	store.set("node1", bundle)

	ch := store.Subscribe("node1")
	defer store.Unsubscribe("node1")

	store.delete("node1")

	select {
	case <-ch:
		// expected
	case <-time.After(1 * time.Second):
		t.Fatal("was not notified")
	}

	_, ok := store.Get("node1")
	assert.False(t, ok)
}

func TestSubscribe_OtherNodes(t *testing.T) {
	store := newTestStore()

	ch := store.Subscribe("node1")
	defer store.Unsubscribe("node1")
	store.Subscribe("node2")
	defer store.Unsubscribe("node2")

	bundle := apiserver.NewMetadataMapperBundle()
	store.set("node2", bundle)

	select {
	case <-ch:
		t.Fatal("node1 subscriber received notification for node2 events")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestUnsubscribe(_ *testing.T) {
	store := newTestStore()

	store.Subscribe("node1")
	store.Unsubscribe("node1")

	bundle := apiserver.NewMetadataMapperBundle()
	bundle.Services.Set("default", "pod1", "svc1")
	store.set("node1", bundle)

	// No subscriber for node1, so this shouldn't do anything. Just testing that
	// this doesn't panic or block.
}

func newTestStore() *MetaBundleStore {
	return &MetaBundleStore{
		cache:       gocache.New(gocache.NoExpiration, 5*time.Second),
		subscribers: make(map[string]chan struct{}),
	}
}
