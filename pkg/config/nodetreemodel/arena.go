// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package nodetreemodel defines a model for the config using a tree of nodes
package nodetreemodel

import "fmt"

var poolInnerNode []innerNode = make([]innerNode, 1024)
var availInnerNode int = 0

var poolLeafNode []leafNodeImpl = make([]leafNodeImpl, 8192)
var availLeafNode int = 0

func allocateNewInnerNode() *innerNode {
	if availInnerNode < len(poolInnerNode) {
		i := availInnerNode
		availInnerNode += 1
		return &poolInnerNode[i]
	}
	obj := innerNode{}
	availInnerNode += 1
	return &obj
}

func allocateNewLeafNode() *leafNodeImpl {
	if availLeafNode < len(poolLeafNode) {
		i := availLeafNode
		availLeafNode += 1
		return &poolLeafNode[i]
	}
	obj := leafNodeImpl{}
	availLeafNode += 1
	return &obj
}

func dumpAllocations() {
	fmt.Printf("========================\n")
	fmt.Printf("========================\n")
	fmt.Printf("========================\n")
	fmt.Printf("= innerNode count = %d\n", availInnerNode)
	fmt.Printf("= leafNode  count = %d\n", availLeafNode)
	fmt.Printf("========================\n")
}
