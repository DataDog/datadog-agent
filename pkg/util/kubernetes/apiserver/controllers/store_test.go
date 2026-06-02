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
	defer store.Unsubscribe("node1", ch)

	bundle := apiserver.NewMetadataMapperBundle()
	bundle.Services.Set("default", "pod1", "svc1")
	store.set("node1", bundle)

	assertNotified(t, "subscriber", ch)
	assertNotNotified(t, "subscriber", ch)
}

func TestSubscribe_WithDelete(t *testing.T) {
	store := newTestStore()

	bundle := apiserver.NewMetadataMapperBundle()
	bundle.Services.Set("default", "pod1", "svc1")
	store.set("node1", bundle)

	ch := store.Subscribe("node1")
	defer store.Unsubscribe("node1", ch)

	store.delete("node1")

	assertNotified(t, "subscriber", ch)

	_, ok := store.Get("node1")
	assert.False(t, ok)
}

func TestSubscribe_OtherNodes(t *testing.T) {
	store := newTestStore()

	ch := store.Subscribe("node1")
	defer store.Unsubscribe("node1", ch)
	ch2 := store.Subscribe("node2")
	defer store.Unsubscribe("node2", ch2)

	bundle := apiserver.NewMetadataMapperBundle()
	store.set("node2", bundle)

	assertNotNotified(t, "node1", ch)
}

func TestUnsubscribe(t *testing.T) {
	t.Run("unsubscribing the older subscriber leaves the newer one working", func(t *testing.T) {
		store := newTestStore()
		olderCh := store.Subscribe("node1")
		newerCh := store.Subscribe("node1")

		store.Unsubscribe("node1", olderCh)
		bundle := apiserver.NewMetadataMapperBundle()
		bundle.Services.Set("default", "pod1", "svc1")
		store.set("node1", bundle)

		assertNotified(t, "newer", newerCh)
		assertNotNotified(t, "older", olderCh)
	})

	t.Run("unsubscribing the newer subscriber leaves the older one working", func(t *testing.T) {
		store := newTestStore()
		olderCh := store.Subscribe("node1")
		newerCh := store.Subscribe("node1")

		store.Unsubscribe("node1", newerCh)
		bundle := apiserver.NewMetadataMapperBundle()
		bundle.Services.Set("default", "pod1", "svc1")
		store.set("node1", bundle)

		assertNotified(t, "older", olderCh)
		assertNotNotified(t, "newer", newerCh)
	})

	t.Run("unsubscribing an unknown channel is a no-op", func(t *testing.T) {
		store := newTestStore()
		olderCh := store.Subscribe("node1")
		newerCh := store.Subscribe("node1")

		unknown := make(chan struct{}, 1)
		store.Unsubscribe("node1", unknown)
		bundle := apiserver.NewMetadataMapperBundle()
		bundle.Services.Set("default", "pod1", "svc1")
		store.set("node1", bundle)

		assertNotified(t, "older", olderCh)
		assertNotified(t, "newer", newerCh)
	})
}

func newTestStore() *MetaBundleStore {
	return &MetaBundleStore{
		cache:       gocache.New(gocache.NoExpiration, 5*time.Second),
		subscribers: make(map[string][]chan struct{}),
	}
}

func assertNotified(t *testing.T, name string, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(1 * time.Second):
		t.Fatalf("%s subscriber was not notified", name)
	}
}

func assertNotNotified(t *testing.T, name string, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("%s subscriber was notified but should not have been", name)
	case <-time.After(50 * time.Millisecond):
	}
}
