// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/types"
)

func TestNewCache(t *testing.T) {
	cache := NewCache(3600)
	require.NotNil(t, cache)
	assert.Equal(t, int64(3600), cache.TTL)
	assert.NotNil(t, cache.Forests)
}

func TestAddParentChildRelation(t *testing.T) {
	cache := NewCache(3600)
	childGVKR, childName := types.GroupVersionResourceKind{Group: "v1", Version: "", Resource: "pods", Kind: "Pod"}, "childPod"
	parentGVKR, parentName := types.GroupVersionResourceKind{Group: "apps", Version: "v1", Resource: "replicaSet", Kind: "ReplicaSet"}, "parentName"
	grandParentGVKR, grandParentName := types.GroupVersionResourceKind{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment"}, "grandParentName"

	cache.AddParentChildRelation("default", parentGVKR, childGVKR, parentName, childName)
	cache.AddParentChildRelation("default", grandParentGVKR, parentGVKR, grandParentName, parentName)

	tree := cache.Forests["default"]
	require.NotNil(t, tree)

	grandParentKey := fmt.Sprintf("%s/%s", grandParentGVKR.Kind, grandParentName)
	parentKey := fmt.Sprintf("%s/%s", parentGVKR.Kind, parentName)
	childKey := fmt.Sprintf("%s/%s", childGVKR.Kind, childName)

	assert.Contains(t, tree.Nodes, parentKey)
	assert.Contains(t, tree.Nodes, childKey)
	assert.Contains(t, tree.Nodes[grandParentKey].Children, parentKey)
	assert.Contains(t, tree.Nodes[parentKey].Parents, grandParentKey)
	assert.Contains(t, tree.Nodes[parentKey].Children, childKey)
	assert.Contains(t, tree.Nodes[childKey].Parents, parentKey)
}

func TestMultipleChildren(t *testing.T) {
	cache := NewCache(3600)
	child1GVKR, child1Name := types.GroupVersionResourceKind{Group: "custom", Version: "v1", Resource: "crd", Kind: "CRD"}, "childCRD1"
	child2GVKR, child2Name := types.GroupVersionResourceKind{Group: "custom", Version: "v1", Resource: "crd", Kind: "CRD"}, "childCRD2"
	parentGVKR, parentName := types.GroupVersionResourceKind{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment"}, "parentName"

	cache.AddParentChildRelation("default", parentGVKR, child1GVKR, parentName, child1Name)
	cache.AddParentChildRelation("default", parentGVKR, child2GVKR, parentName, child2Name)

	tree := cache.Forests["default"]
	require.NotNil(t, tree)

	parentKey := fmt.Sprintf("%s/%s", parentGVKR.Kind, parentName)
	child1Key := fmt.Sprintf("%s/%s", child1GVKR.Kind, child1Name)
	child2Key := fmt.Sprintf("%s/%s", child2GVKR.Kind, child2Name)

	assert.Contains(t, tree.Nodes, parentKey)
	assert.Contains(t, tree.Nodes, child1Key)
	assert.Contains(t, tree.Nodes, child2Key)

	assert.Contains(t, tree.Nodes[parentKey].Children, child1Key)
	assert.Contains(t, tree.Nodes[parentKey].Children, child2Key)
}

func TestGetParentTree(t *testing.T) {
	cache := NewCache(3600)
	childGVKR, childName := types.GroupVersionResourceKind{Group: "", Version: "v1", Resource: "pods", Kind: "Pod"}, "childPod"
	parentGVKR, parentName := types.GroupVersionResourceKind{Group: "apps", Version: "v1", Resource: "replicaSet", Kind: "ReplicaSet"}, "parentName"
	grandParentGVKR, grandParentName := types.GroupVersionResourceKind{Group: "custom", Version: "v1", Resource: "deployments", Kind: "Deployment"}, "grandParentName"
	friendGVKR, friendName := types.GroupVersionResourceKind{Group: "batch", Version: "v1", Resource: "deployments", Kind: "Deployment"}, "friendName"

	cache.AddParentChildRelation("default", parentGVKR, childGVKR, parentName, childName)
	cache.AddParentChildRelation("default", grandParentGVKR, parentGVKR, grandParentName, parentName)
	cache.AddSingleParent("default", friendGVKR, friendName)

	tree := cache.GetParentTree("default", childGVKR.Kind, childName)

	require.NotNil(t, tree)
	assert.Len(t, tree, 3)

	assert.Contains(t, tree, Item{GVKR: parentGVKR, Name: parentName})
	assert.Contains(t, tree, Item{GVKR: childGVKR, Name: childName})
	assert.Contains(t, tree, Item{GVKR: grandParentGVKR, Name: grandParentName})

	tree = cache.GetParentTree("default", friendGVKR.Kind, friendName)

	require.NotNil(t, tree)
	assert.Len(t, tree, 1)

	// Independent namespace

	cache.AddParentChildRelation("agent", parentGVKR, childGVKR, parentName, childName)

	tree = cache.GetParentTree("agent", childGVKR.Kind, childName)
	require.NotNil(t, tree)
	assert.Len(t, tree, 2)
}

func TestToJSONAndFromJSON(t *testing.T) {
	cache := NewCache(3600)
	parentGVKR, parentName := types.GroupVersionResourceKind{Group: "", Version: "v1", Resource: "deployments", Kind: "Deployment"}, "parentName"
	childGVKR, childName := types.GroupVersionResourceKind{Group: "apps", Version: "v1", Resource: "pods", Kind: "Pod"}, "childName"

	cache.AddParentChildRelation("default", parentGVKR, childGVKR, parentName, childName)
	cache.AddParentChildRelation("agent", parentGVKR, childGVKR, parentName, childName)

	jsonData, err := cache.ToJSON()
	require.NoError(t, err)
	require.NotEmpty(t, jsonData)

	newCache := NewCache(3600)
	err = newCache.FromJSON(jsonData)
	require.NoError(t, err)

	tree := newCache.GetParentTree("default", childGVKR.Kind, childName)
	require.NotNil(t, tree)
	assert.Len(t, tree, 2)

	tree = newCache.GetParentTree("agent", childGVKR.Kind, childName)
	require.NotNil(t, tree)
	assert.Len(t, tree, 2)

}
