// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package store provides a tree data structure for storing parent-child relationships.
package store

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	k8stypes "github.com/DataDog/datadog-agent/pkg/util/kubernetes/types"
)

// Cache is a map of trees, keyed by the namespace.
type Cache struct {
	Forests map[string]*Forest
	TTL     int64
}

// NewCache initializes and returns a new cache.
func NewCache(ttl int64) *Cache {
	return &Cache{TTL: ttl, Forests: make(map[string]*Forest)}
}

// AddParentChildRelation adds a parent-child relationship between two nodes in the cache.
func (c *Cache) AddParentChildRelation(namespace string, parentGVKR, childGVKR k8stypes.GroupVersionResourceKind, parentName, childName string) {
	if _, exists := c.Forests[namespace]; !exists {
		c.Forests[namespace] = NewForest(c.TTL)
	}
	c.Forests[namespace].AddParentChildRelation(parentGVKR, childGVKR, parentName, childName)
}

// AddSingleParent adds a single parent to the cache.
// (Eg. An independent replicaSet with no parent deployment)
func (c *Cache) AddSingleParent(namespace string, gvkr k8stypes.GroupVersionResourceKind, name string) {
	if _, exists := c.Forests[namespace]; !exists {
		c.Forests[namespace] = NewForest(c.TTL)
	}
	c.Forests[namespace].getOrAddNode(gvkr, name)
}

// GetParentTree returns the Tree related to the specific leaf node's parent.
func (c *Cache) GetParentTree(namespace, childKind, childName string) []Item {
	forest := c.Forests[namespace]
	if forest == nil {
		return nil
	}
	return forest.GetTree(childKind, childName)
}

// AddParentTree adds a list of parent-child relationships to the cache.
func (c *Cache) AddParentTree(namespace string, edges []k8stypes.ObjectRelation) {
	for _, edge := range edges {
		c.AddParentChildRelation(namespace, edge.ParentGVRK, edge.ChildGVRK, edge.ParentName, edge.ChildName)
	}
}

// ToJSON converts the cache to a JSON string for storage.
func (c *Cache) ToJSON() ([]byte, error) {
	jsonData, err := json.Marshal(c.Forests)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cache: %w", err)
	}
	return jsonData, nil
}

// FromJSON loads a cache from a JSON string.
func (c *Cache) FromJSON(data []byte) error {
	if err := json.Unmarshal(data, &c.Forests); err != nil {
		return fmt.Errorf("failed to unmarshal cache: %w", err)
	}
	return nil
}

// CleanCache removes all nodes that have timed out
func (c *Cache) CleanCache() {
	for _, forest := range c.Forests {
		forest.RemoveExpiredNodes()
	}
}

// Node represents a tree node with GVK, name, parents, and children.
type Node struct {
	Item       Item     `json:"item"`
	Parents    []string `json:"parents"`  // Store parent node keys
	Children   []string `json:"children"` // Store child node keys
	LastAccess int64    `json:"last_access"`
	Count      int      `json:"count"`
}

// Item represents a node in the tree.
type Item struct {
	GVKR k8stypes.GroupVersionResourceKind `json:"gvkr"`
	Name string                            `json:"name"`
}

// Forest represents the entire tree, storing nodes in a map for O(1) access.
type Forest struct {
	Nodes map[string]*Node `json:"nodes"`
	TTL   int64
	mu    sync.Mutex
}

// NewForest initializes and returns a new tree.
func NewForest(ttl int64) *Forest {
	return &Forest{
		Nodes: make(map[string]*Node),
		TTL:   ttl,
	}
}

// getOrAddNode adds a node to the tree if it doesn't exist
func (t *Forest) getOrAddNode(gvkr k8stypes.GroupVersionResourceKind, name string) *Node {
	key := fmt.Sprintf("%s/%s", gvkr.Kind, name)
	if _, exists := t.Nodes[key]; !exists {
		t.Nodes[key] = &Node{
			Item: Item{
				GVKR: gvkr,
				Name: name,
			},
			Parents:    []string{},
			Children:   []string{},
			LastAccess: time.Now().Unix(),
		}
	} else {
		t.Nodes[key].LastAccess = time.Now().Unix()
	}
	return t.Nodes[key]
}

// AddParentChildRelation adds a parent-child relationship between two nodes.
func (t *Forest) AddParentChildRelation(parentGVKR, childGVKR k8stypes.GroupVersionResourceKind, parentName, childName string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	parentKey := fmt.Sprintf("%s/%s", parentGVKR.Kind, parentName)
	childKey := fmt.Sprintf("%s/%s", childGVKR.Kind, childName)

	parentNode := t.getOrAddNode(parentGVKR, parentName)
	childNode := t.getOrAddNode(childGVKR, childName)

	// Add relationship
	parentNode.Children = append(parentNode.Children, childKey)
	childNode.Parents = append(childNode.Parents, parentKey)
}

// GetTree returns the tree starting from the specified node.
func (t *Forest) GetTree(kind, name string) []Item {
	t.mu.Lock()
	defer t.mu.Unlock()

	tree := []Item{}

	queue := []string{}
	queue = append(queue, fmt.Sprintf("%s/%s", kind, name))

	for len(queue) > 0 {
		nodeKey := queue[0]
		queue = queue[1:]

		node, exists := t.Nodes[nodeKey]
		if !exists {
			continue
		}

		tree = append(tree, node.Item)
		queue = append(queue, node.Parents...)
	}

	return tree
}

// RemoveExpiredNodes removes nodes whose TTL has expired.
func (t *Forest) RemoveExpiredNodes() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now().Unix()
	for key, node := range t.Nodes {
		if t.TTL > 0 && now-node.LastAccess > t.TTL {
			delete(t.Nodes, key)
		}
	}
}
