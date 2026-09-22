// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

// PodWatchScope describes the set of nodes whose pods must be watches,
// and provides a way to subscribe to changes to that set.
type PodWatchScope interface {
	// Nodes returns the node names that are owned by this replica.
	Nodes() []string
	// Subscribe registers a callback to be called when the set of nodes owned by this replica changes,
	// and returns a function to unsubscribe.
	Subscribe(fn func()) (unsubscribe func())
}

// NodeSyncReporter is responsible for reporting the sync state of the pods on a node.
type NodeSyncReporter interface {
	NodeSynced(node string, synced bool)
}
