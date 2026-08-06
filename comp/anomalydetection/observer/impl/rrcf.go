// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"math/rand"
)

// node is the interface implemented by both branch and leaf nodes in an RCTree.
type node interface {
	isNode()
	getParent() node
	setParent(node)
	leafCount() int
	setLeafCount(int)
}

// branch is an internal node in a random cut tree.
type branch struct {
	q int       // cut dimension
	p float64   // cut value
	l node      // left child
	r node      // right child
	u node      // parent (nil for root)
	n int       // number of leaves under this branch
	b []float64 // bounding box: [min_d0, ..., min_{ndim-1}, max_d0, ..., max_{ndim-1}]
}

func (b *branch) isNode()            {}
func (b *branch) getParent() node    { return b.u }
func (b *branch) setParent(p node)   { b.u = p }
func (b *branch) leafCount() int     { return b.n }
func (b *branch) setLeafCount(n int) { b.n = n }

// leaf is a terminal node in a random cut tree.
type leaf struct {
	i int       // index/identifier
	d int       // depth in tree
	u node      // parent (nil if leaf is root)
	x []float64 // the point (ndim values)
	n int       // count (for duplicate handling, always 1 in our use case)
}

func (l *leaf) isNode()            {}
func (l *leaf) getParent() node    { return l.u }
func (l *leaf) setParent(p node)   { l.u = p }
func (l *leaf) leafCount() int     { return l.n }
func (l *leaf) setLeafCount(n int) { l.n = n }

// rcTree is a single random cut tree.
type rcTree struct {
	root   node
	leaves map[int]*leaf // index -> leaf pointer
	ndim   int           // dimensionality of points
	rng    *rand.Rand    // random number generator
}

// newRCTree creates an empty random cut tree.
func newRCTree(rng *rand.Rand) *rcTree {
	return &rcTree{
		root:   nil,
		leaves: make(map[int]*leaf),
		ndim:   0,
		rng:    rng,
	}
}

// insertPoint inserts a point into the tree with the given index.
// Returns the created leaf.
func (t *rcTree) insertPoint(point []float64, index int) *leaf {
	// Validate dimensions match if tree is non-empty
	if t.root != nil && len(point) != t.ndim {
		panic("point dimension mismatch")
	}
	// Check index doesn't already exist
	if _, exists := t.leaves[index]; exists {
		panic("index already exists in tree")
	}

	// If tree is empty, create leaf as root
	if t.root == nil {
		newLeaf := &leaf{
			i: index,
			d: 0,
			u: nil,
			x: point,
			n: 1,
		}
		t.root = newLeaf
		t.ndim = len(point)
		t.leaves[index] = newLeaf
		return newLeaf
	}

	// Tree has existing nodes; insert using the RRCF algorithm
	currentNode := t.root
	var parent node
	var side byte // 'l' or 'r' to track which child of parent we descended from
	depth := 0

	// Walk down tree, at each step decide if we create a cut above the current node
	for {
		// RRCF represents identical points as one leaf with multiplicity. Log
		// count series commonly produce long runs of identical shingles; making
		// a zero-width branch for every duplicate gives those normal shingles
		// the same large CoDisp score as a real outlier.
		if leafNode, ok := currentNode.(*leaf); ok && equalPoint(leafNode.x, point) {
			leafNode.n++
			t.updateLeafCountUpwards(leafNode.u, 1)
			t.leaves[index] = leafNode
			return leafNode
		}

		cutDim, cutVal := t.insertPointCutNode(point, currentNode)
		minVal, maxVal := nodeBoundsAt(currentNode, cutDim, t.ndim)

		// If cut separates point from current subtree (cut <= min or cut >= max),
		// create a new branch here
		if cutVal <= minVal {
			// Point goes left, current subtree goes right
			newLeaf := &leaf{
				i: index,
				d: depth,
				u: nil,
				x: point,
				n: 1,
			}
			newBranch := &branch{
				q: cutDim,
				p: cutVal,
				l: newLeaf,
				r: currentNode,
				u: parent,
				n: newLeaf.n + currentNode.leafCount(),
			}
			newLeaf.u = newBranch
			currentNode.setParent(newBranch)

			t.linkBranchToParent(newBranch, parent, side)
			t.incrementDepthsBelow(currentNode, 1)
			t.updateLeafCountUpwards(parent, 1)
			t.tightenBboxUpwards(newBranch)
			t.leaves[index] = newLeaf
			return newLeaf

		} else if cutVal >= maxVal {
			// Current subtree goes left, point goes right
			newLeaf := &leaf{
				i: index,
				d: depth,
				u: nil,
				x: point,
				n: 1,
			}
			newBranch := &branch{
				q: cutDim,
				p: cutVal,
				l: currentNode,
				r: newLeaf,
				u: parent,
				n: newLeaf.n + currentNode.leafCount(),
			}
			newLeaf.u = newBranch
			currentNode.setParent(newBranch)

			t.linkBranchToParent(newBranch, parent, side)
			t.incrementDepthsBelow(currentNode, 1)
			t.updateLeafCountUpwards(parent, 1)
			t.tightenBboxUpwards(newBranch)
			t.leaves[index] = newLeaf
			return newLeaf
		}

		// Cut is inside current node's bbox; descend into tree
		depth++
		br, ok := currentNode.(*branch)
		if !ok {
			// Current node is a leaf but we didn't cut above it - this shouldn't happen
			// with correct algorithm, but let's handle it defensively
			panic("reached leaf without making a cut - algorithm error")
		}

		// Descend based on cut dimension/value of the current branch
		parent = br
		if point[br.q] <= br.p {
			currentNode = br.l
			side = 'l'
		} else {
			currentNode = br.r
			side = 'r'
		}
	}
}

func equalPoint(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// linkBranchToParent links a new branch to its parent (or sets it as root).
func (t *rcTree) linkBranchToParent(newBranch *branch, parent node, side byte) {
	if parent == nil {
		t.root = newBranch
	} else {
		parentBranch := parent.(*branch)
		if side == 'l' {
			parentBranch.l = newBranch
		} else {
			parentBranch.r = newBranch
		}
	}
}

// insertPointCut generates cut dimension and value for inserting a new point.
// bbox is the bounding box of the current subtree in flat format.
func (t *rcTree) insertPointCut(point []float64, bbox []float64) (cutDim int, cutVal float64) {
	return t.insertPointCutFromBounds(point, bbox, nil)
}

func (t *rcTree) insertPointCutNode(point []float64, current node) (cutDim int, cutVal float64) {
	return t.insertPointCutFromBounds(point, nil, current)
}

func (t *rcTree) insertPointCutFromBounds(point, bbox []float64, current node) (cutDim int, cutVal float64) {
	totalRange := 0.0
	for d := 0; d < t.ndim; d++ {
		minVal, maxVal := t.boundsAt(bbox, current, d)
		totalRange += math.Max(maxVal, point[d]) - math.Min(minVal, point[d])
	}

	// If all spans are zero (all points identical), pick dimension 0 arbitrarily
	if totalRange == 0 {
		return 0, point[0]
	}

	// Choose cut point uniformly over total range
	r := t.rng.Float64() * totalRange

	// Find which dimension the cut falls in
	cumSum := 0.0
	cutDim = t.ndim - 1 // default to last dimension
	for d := 0; d < t.ndim; d++ {
		minVal, maxVal := t.boundsAt(bbox, current, d)
		cumSum += math.Max(maxVal, point[d]) - math.Min(minVal, point[d])
		if cumSum >= r {
			cutDim = d
			break
		}
	}

	// Cut value within that dimension
	minVal, _ := t.boundsAt(bbox, current, cutDim)
	cutVal = math.Min(minVal, point[cutDim]) + cumSum - r

	return cutDim, cutVal
}

func (t *rcTree) boundsAt(bbox []float64, current node, d int) (float64, float64) {
	if current != nil {
		return nodeBoundsAt(current, d, t.ndim)
	}
	return bbox[d], bbox[t.ndim+d]
}

func nodeBoundsAt(n node, d, ndim int) (float64, float64) {
	switch current := n.(type) {
	case *leaf:
		return current.x[d], current.x[d]
	case *branch:
		return current.b[d], current.b[ndim+d]
	default:
		panic("unknown RRCF node type")
	}
}

// forgetPoint removes a point from the tree by its index.
// Returns the removed leaf.
func (t *rcTree) forgetPoint(index int) *leaf {
	leafNode, exists := t.leaves[index]
	if !exists {
		panic("index does not exist in tree")
	}

	// Handle duplicate counts (not used in our case, but keeping for completeness)
	if leafNode.n > 1 {
		t.updateLeafCountUpwards(leafNode, -1)
		delete(t.leaves, index)
		return leafNode
	}

	// If leaf is root, tree becomes empty
	if leafNode == t.root {
		t.root = nil
		t.ndim = 0
		delete(t.leaves, index)
		return leafNode
	}

	// Find parent and sibling
	parent := leafNode.u.(*branch)
	var sibling node
	if leafNode == parent.l {
		sibling = parent.r
	} else {
		sibling = parent.l
	}

	// If parent is root, sibling becomes new root
	if parent == t.root {
		sibling.setParent(nil)
		t.root = sibling
		t.incrementDepthsBelow(sibling, -1)
		delete(t.leaves, index)
		return leafNode
	}

	// General case: short-circuit grandparent to sibling
	grandparent := parent.u.(*branch)
	sibling.setParent(grandparent)

	if parent == grandparent.l {
		grandparent.l = sibling
	} else {
		grandparent.r = sibling
	}

	// Update depths below sibling (they decrease by 1)
	t.incrementDepthsBelow(sibling, -1)

	// Update leaf counts upwards from grandparent
	t.updateLeafCountUpwards(grandparent, -1)

	// Update bounding boxes upwards
	t.relaxBboxUpwards(grandparent, leafNode.x)

	delete(t.leaves, index)
	return leafNode
}

// disp computes the displacement score for a leaf.
// Displacement is the number of leaves in the sibling subtree.
func (t *rcTree) disp(index int) int {
	leafNode, exists := t.leaves[index]
	if !exists {
		panic("index does not exist in tree")
	}

	// If leaf is root, displacement is 0
	if leafNode == t.root {
		return 0
	}

	parent := leafNode.u.(*branch)
	var sibling node
	if leafNode == parent.l {
		sibling = parent.r
	} else {
		sibling = parent.l
	}

	return sibling.leafCount()
}

// codisp computes the collusive displacement score for a leaf.
// This is the primary anomaly score in RRCF - higher means more anomalous.
func (t *rcTree) codisp(index int) float64 {
	leafNode, exists := t.leaves[index]
	if !exists {
		panic("index does not exist in tree")
	}

	// If leaf is root, codisp is 0
	if leafNode == t.root {
		return 0
	}

	var maxResult float64
	var currentNode node = leafNode

	// Walk from leaf to root, computing displacement ratio at each level
	for currentNode.getParent() != nil {
		parent := currentNode.getParent().(*branch)

		var sibling node
		if currentNode == parent.l {
			sibling = parent.r
		} else {
			sibling = parent.l
		}

		// Ratio: how many points would be displaced if we removed this subtree
		numDeleted := currentNode.leafCount()
		displacement := sibling.leafCount()
		result := float64(displacement) / float64(numDeleted)

		if result > maxResult {
			maxResult = result
		}

		currentNode = parent
	}

	return maxResult
}

// updateLeafCountUpwards updates leaf counts going up the tree.
func (t *rcTree) updateLeafCountUpwards(n node, inc int) {
	for n != nil {
		n.setLeafCount(n.leafCount() + inc)
		n = n.getParent()
	}
}

// incrementDepthsBelow recursively increments depth of all leaves below a node.
func (t *rcTree) incrementDepthsBelow(n node, inc int) {
	if leafNode, ok := n.(*leaf); ok {
		leafNode.d += inc
		return
	}

	br := n.(*branch)
	t.incrementDepthsBelow(br.l, inc)
	t.incrementDepthsBelow(br.r, inc)
}

// computeBranchBbox computes bounding box of a branch from its children.
func (t *rcTree) computeBranchBbox(br *branch) []float64 {
	return t.computeBranchBboxInto(br, nil)
}

func (t *rcTree) computeBranchBboxInto(br *branch, bbox []float64) []float64 {
	if cap(bbox) < 2*t.ndim {
		bbox = make([]float64, 2*t.ndim)
	} else {
		bbox = bbox[:2*t.ndim]
	}

	for d := 0; d < t.ndim; d++ {
		leftMin, leftMax := nodeBoundsAt(br.l, d, t.ndim)
		rightMin, rightMax := nodeBoundsAt(br.r, d, t.ndim)
		bbox[d] = math.Min(leftMin, rightMin)
		bbox[t.ndim+d] = math.Max(leftMax, rightMax)
	}

	return bbox
}

// tightenBboxUpwards expands bounding boxes going up after insertion.
func (t *rcTree) tightenBboxUpwards(br *branch) {
	// Set bbox of the new branch
	bbox := t.computeBranchBbox(br)
	br.b = bbox

	// Walk up and expand parent bboxes if needed
	n := br.getParent()
	for n != nil {
		parentBranch := n.(*branch)
		parentBbox := parentBranch.b
		if parentBbox == nil {
			parentBranch.b = t.computeBranchBbox(parentBranch)
			n = n.getParent()
			continue
		}

		changed := false
		ndim := t.ndim

		// Check if we need to expand mins
		for d := 0; d < ndim; d++ {
			if bbox[d] < parentBbox[d] {
				parentBbox[d] = bbox[d]
				changed = true
			}
		}
		// Check if we need to expand maxes
		for d := 0; d < ndim; d++ {
			if bbox[ndim+d] > parentBbox[ndim+d] {
				parentBbox[ndim+d] = bbox[ndim+d]
				changed = true
			}
		}

		if !changed {
			break // No need to continue if this bbox didn't change
		}

		n = n.getParent()
	}
}

// relaxBboxUpwards contracts bounding boxes going up after deletion.
func (t *rcTree) relaxBboxUpwards(n node, deletedPoint []float64) {
	for n != nil {
		br := n.(*branch)
		oldBbox := br.b

		// Check if deleted point was on the boundary
		onBoundary := false
		ndim := t.ndim
		for d := 0; d < ndim; d++ {
			if deletedPoint[d] == oldBbox[d] || deletedPoint[d] == oldBbox[ndim+d] {
				onBoundary = true
				break
			}
		}

		if !onBoundary {
			break // Point wasn't on boundary, bbox unchanged
		}

		// Recompute bbox from children
		br.b = t.computeBranchBboxInto(br, br.b)
		n = n.getParent()
	}
}

// rcForest is a collection of random cut trees for anomaly detection.
type rcForest struct {
	trees      []*rcTree
	numTrees   int
	treeSize   int // max points per tree
	ndim       int
	rng        *rand.Rand
	seed       int64
	indexQueue []int // FIFO queue of point indices for sliding window
	nextIndex  int   // next index to assign to new points
}

// newRCForest creates a new forest with the given parameters.
func newRCForest(numTrees, treeSize, ndim int, seed int64) *rcForest {
	rng := rand.New(rand.NewSource(seed))

	trees := make([]*rcTree, numTrees)
	for i := 0; i < numTrees; i++ {
		// Each tree gets its own RNG seeded from the main RNG
		treeSeed := rng.Int63()
		treeRng := rand.New(rand.NewSource(treeSeed))
		trees[i] = newRCTree(treeRng)
	}

	return &rcForest{
		trees:      trees,
		numTrees:   numTrees,
		treeSize:   treeSize,
		ndim:       ndim,
		rng:        rng,
		seed:       seed,
		indexQueue: make([]int, 0, treeSize),
		nextIndex:  0,
	}
}

// insertPoint inserts a point into all trees and returns the index and average CoDisp score.
// If at capacity, the oldest point is evicted first.
func (f *rcForest) insertPoint(point []float64) (index int, avgCodisp float64) {
	if len(point) != f.ndim {
		panic("point dimension mismatch with forest")
	}

	// Evict oldest point if at capacity
	if len(f.indexQueue) >= f.treeSize {
		oldestIdx := f.indexQueue[0]
		f.indexQueue = f.indexQueue[1:]
		for _, tree := range f.trees {
			tree.forgetPoint(oldestIdx)
		}
	}

	// Insert point into all trees
	index = f.nextIndex
	f.nextIndex++

	for _, tree := range f.trees {
		tree.insertPoint(point, index)
	}

	f.indexQueue = append(f.indexQueue, index)

	// Compute average CoDisp across all trees
	totalCodisp := 0.0
	for _, tree := range f.trees {
		totalCodisp += tree.codisp(index)
	}
	avgCodisp = totalCodisp / float64(f.numTrees)

	return index, avgCodisp
}

// score computes the anomaly score for a point without permanently adding it.
// Inserts the point, computes score, then removes it.
// Note: This does NOT evict old points, use insertPoint for streaming.
func (f *rcForest) score(point []float64) float64 {
	if len(point) != f.ndim {
		panic("point dimension mismatch with forest")
	}

	// Use a temporary index that won't collide
	tempIndex := -1 - f.nextIndex

	// Insert into all trees
	for _, tree := range f.trees {
		tree.insertPoint(point, tempIndex)
	}

	// Compute average CoDisp
	totalCodisp := 0.0
	for _, tree := range f.trees {
		totalCodisp += tree.codisp(tempIndex)
	}
	avgCodisp := totalCodisp / float64(f.numTrees)

	// Remove from all trees
	for _, tree := range f.trees {
		tree.forgetPoint(tempIndex)
	}

	return avgCodisp
}

// reset clears all trees in the forest.
func (f *rcForest) reset() {
	// Recreate the seed stream as well as the trees so Reset and a fresh
	// detector produce identical scores for the same replay.
	f.rng = rand.New(rand.NewSource(f.seed))
	for i := range f.trees {
		treeSeed := f.rng.Int63()
		treeRng := rand.New(rand.NewSource(treeSeed))
		f.trees[i] = newRCTree(treeRng)
	}
	f.indexQueue = f.indexQueue[:0]
	f.nextIndex = 0
}

// size returns the current number of points in the forest.
func (f *rcForest) size() int {
	return len(f.indexQueue)
}
