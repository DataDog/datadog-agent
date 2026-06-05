// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"math/rand"
	"testing"
)

func TestRCTree_EmptyTree(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := newRCTree(rng)

	if tree.root != nil {
		t.Error("expected nil root for empty tree")
	}
	if len(tree.leaves) != 0 {
		t.Error("expected no leaves for empty tree")
	}
}

func TestRCTree_InsertSinglePoint(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := newRCTree(rng)

	point := []float64{1.0, 2.0, 3.0}
	leaf := tree.insertPoint(point, 0)

	if tree.root != leaf {
		t.Error("single point should be root")
	}
	if tree.ndim != 3 {
		t.Errorf("expected ndim=3, got %d", tree.ndim)
	}
	if leaf.n != 1 {
		t.Errorf("expected leaf count=1, got %d", leaf.n)
	}
	if leaf.d != 0 {
		t.Errorf("expected depth=0, got %d", leaf.d)
	}

	// Verify the point is stored correctly
	for i, v := range point {
		if leaf.x[i] != v {
			t.Errorf("point mismatch at index %d: expected %f, got %f", i, v, leaf.x[i])
		}
	}

	// Check leaves map
	if tree.leaves[0] != leaf {
		t.Error("leaf not in leaves map")
	}
}

func TestRCTree_InsertMultiplePoints(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := newRCTree(rng)

	points := [][]float64{
		{1.0, 2.0},
		{3.0, 4.0},
		{5.0, 6.0},
	}

	for i, p := range points {
		tree.insertPoint(p, i)
	}

	// Verify all points are in the tree
	if len(tree.leaves) != 3 {
		t.Errorf("expected 3 leaves, got %d", len(tree.leaves))
	}

	// Root should be a branch with 3 leaves under it
	br, ok := tree.root.(*branch)
	if !ok {
		// If root is a leaf, that's wrong
		if _, isLeaf := tree.root.(*leaf); isLeaf && len(points) > 1 {
			t.Error("root should be branch with multiple points")
		}
	} else {
		if br.n != 3 {
			t.Errorf("expected root branch leaf count=3, got %d", br.n)
		}
	}
}

func TestRCTree_ForgetPoint(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := newRCTree(rng)

	// Insert points
	tree.insertPoint([]float64{1.0, 2.0}, 0)
	tree.insertPoint([]float64{3.0, 4.0}, 1)
	tree.insertPoint([]float64{5.0, 6.0}, 2)

	// Forget middle point
	tree.forgetPoint(1)

	if len(tree.leaves) != 2 {
		t.Errorf("expected 2 leaves after forget, got %d", len(tree.leaves))
	}
	if _, exists := tree.leaves[1]; exists {
		t.Error("forgotten leaf should not be in leaves map")
	}

	// Root should still have correct count
	if tree.root.leafCount() != 2 {
		t.Errorf("expected root leaf count=2, got %d", tree.root.leafCount())
	}
}

func TestRCTree_ForgetOnlyPoint(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := newRCTree(rng)

	tree.insertPoint([]float64{1.0, 2.0}, 0)
	tree.forgetPoint(0)

	if tree.root != nil {
		t.Error("expected nil root after forgetting only point")
	}
	if len(tree.leaves) != 0 {
		t.Error("expected no leaves after forgetting only point")
	}
}

func TestRCTree_Disp(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := newRCTree(rng)

	// Single point - displacement should be 0
	tree.insertPoint([]float64{1.0, 2.0}, 0)
	disp := tree.disp(0)
	if disp != 0 {
		t.Errorf("single point disp should be 0, got %d", disp)
	}

	// Add more points
	tree.insertPoint([]float64{100.0, 100.0}, 1) // outlier

	// Displacement of the outlier should be 1 (the original point is the sibling)
	disp = tree.disp(1)
	if disp != 1 {
		t.Errorf("outlier disp should be 1, got %d", disp)
	}
}

func TestRCTree_Codisp(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := newRCTree(rng)

	// Build a tree with some normal points and an outlier
	normalPoints := [][]float64{
		{0.1, 0.1},
		{0.2, 0.2},
		{0.3, 0.3},
		{0.4, 0.4},
	}

	for i, p := range normalPoints {
		tree.insertPoint(p, i)
	}

	// Add an outlier
	tree.insertPoint([]float64{100.0, 100.0}, 100)

	// The outlier should have a higher codisp than normal points
	outlierCodisp := tree.codisp(100)

	// Codisp should be positive
	if outlierCodisp <= 0 {
		t.Errorf("outlier codisp should be positive, got %f", outlierCodisp)
	}

	// At minimum, outlier codisp should be number of other points / 1 = 4
	// (since removing the outlier would displace at least some of the normal points)
	if outlierCodisp < 1.0 {
		t.Errorf("outlier codisp should be >= 1, got %f", outlierCodisp)
	}
}

func TestRCForest_Basic(t *testing.T) {
	forest := newRCForest(10, 50, 3, 42)

	if forest.size() != 0 {
		t.Error("new forest should be empty")
	}

	// Insert a point
	idx, score := forest.insertPoint([]float64{1.0, 2.0, 3.0})
	if idx != 0 {
		t.Errorf("first index should be 0, got %d", idx)
	}
	if forest.size() != 1 {
		t.Errorf("forest size should be 1, got %d", forest.size())
	}

	// First point in empty tree has codisp 0
	if score != 0 {
		t.Errorf("first point score should be 0, got %f", score)
	}

	// Insert more points
	for i := 1; i < 10; i++ {
		forest.insertPoint([]float64{float64(i), float64(i), float64(i)})
	}

	if forest.size() != 10 {
		t.Errorf("forest size should be 10, got %d", forest.size())
	}
}

func TestRCForest_SlidingWindow(t *testing.T) {
	// Small tree size to test eviction
	forest := newRCForest(3, 5, 2, 42)

	// Insert 5 points to fill the window
	for i := 0; i < 5; i++ {
		forest.insertPoint([]float64{float64(i), float64(i)})
	}

	if forest.size() != 5 {
		t.Errorf("expected size 5, got %d", forest.size())
	}

	// Insert one more - should evict the oldest
	forest.insertPoint([]float64{5.0, 5.0})

	if forest.size() != 5 {
		t.Errorf("expected size 5 after eviction, got %d", forest.size())
	}

	// Verify first tree doesn't have index 0 anymore
	if _, exists := forest.trees[0].leaves[0]; exists {
		t.Error("index 0 should have been evicted")
	}

	// But should have indices 1-5
	for i := 1; i <= 5; i++ {
		if _, exists := forest.trees[0].leaves[i]; !exists {
			t.Errorf("index %d should exist", i)
		}
	}
}

func TestRCForest_OutlierDetection(t *testing.T) {
	// Larger forest for more reliable scoring
	forest := newRCForest(50, 100, 2, 42)

	// Insert many normal points clustered around origin
	for i := 0; i < 50; i++ {
		x := float64(i%10) * 0.1
		y := float64(i/10) * 0.1
		forest.insertPoint([]float64{x, y})
	}

	// Insert an outlier far from the cluster
	_, outlierScore := forest.insertPoint([]float64{100.0, 100.0})

	// Insert another normal point
	_, normalScore := forest.insertPoint([]float64{0.5, 0.5})

	// Outlier should have higher score than normal point
	if outlierScore <= normalScore {
		t.Errorf("outlier score (%f) should be higher than normal score (%f)",
			outlierScore, normalScore)
	}
}

func TestRCForest_Score(t *testing.T) {
	forest := newRCForest(10, 50, 2, 42)

	// Build up the forest
	for i := 0; i < 20; i++ {
		forest.insertPoint([]float64{float64(i % 5), float64(i / 5)})
	}

	sizeBefore := forest.size()

	// Score a point without adding it
	score := forest.score([]float64{100.0, 100.0})

	// Score should be positive for outlier
	if score <= 0 {
		t.Errorf("outlier score should be positive, got %f", score)
	}

	// Size should be unchanged
	if forest.size() != sizeBefore {
		t.Errorf("forest size should be unchanged after score(), was %d, now %d",
			sizeBefore, forest.size())
	}
}

func TestRCForest_Reset(t *testing.T) {
	forest := newRCForest(5, 20, 2, 42)

	// Insert some points
	for i := 0; i < 10; i++ {
		forest.insertPoint([]float64{float64(i), float64(i)})
	}

	if forest.size() != 10 {
		t.Errorf("expected size 10, got %d", forest.size())
	}

	// Reset
	forest.reset()

	if forest.size() != 0 {
		t.Errorf("expected size 0 after reset, got %d", forest.size())
	}

	// Should be able to insert again
	forest.insertPoint([]float64{1.0, 2.0})
	if forest.size() != 1 {
		t.Errorf("expected size 1 after re-insert, got %d", forest.size())
	}
}

func TestRCTree_BoundingBoxes(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := newRCTree(rng)

	points := [][]float64{
		{0.0, 0.0},
		{1.0, 1.0},
		{0.5, 0.5},
	}

	for i, p := range points {
		tree.insertPoint(p, i)
	}

	// Root should be a branch
	br, ok := tree.root.(*branch)
	if !ok {
		t.Fatal("root should be a branch")
	}

	// Check bounding box of root
	bbox := br.b
	ndim := tree.ndim

	// Min should be (0, 0)
	for d := 0; d < ndim; d++ {
		if bbox[d] != 0.0 {
			t.Errorf("min[%d] should be 0, got %f", d, bbox[d])
		}
	}

	// Max should be (1, 1)
	for d := 0; d < ndim; d++ {
		if bbox[ndim+d] != 1.0 {
			t.Errorf("max[%d] should be 1, got %f", d, bbox[ndim+d])
		}
	}
}

func TestRCTree_DimensionValidation(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := newRCTree(rng)

	tree.insertPoint([]float64{1.0, 2.0}, 0)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for dimension mismatch")
		}
	}()

	// This should panic - wrong dimension
	tree.insertPoint([]float64{1.0, 2.0, 3.0}, 1)
}

func TestRCTree_DuplicateIndexValidation(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := newRCTree(rng)

	tree.insertPoint([]float64{1.0, 2.0}, 0)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for duplicate index")
		}
	}()

	// This should panic - duplicate index
	tree.insertPoint([]float64{3.0, 4.0}, 0)
}

func TestRCTree_ForgetNonexistentValidation(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := newRCTree(rng)

	tree.insertPoint([]float64{1.0, 2.0}, 0)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nonexistent index")
		}
	}()

	// This should panic - index doesn't exist
	tree.forgetPoint(999)
}

// TestRCTree_ManyInsertDelete tests the tree structure remains valid after many operations
func TestRCTree_ManyInsertDelete(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := newRCTree(rng)

	// Insert 100 points
	for i := 0; i < 100; i++ {
		tree.insertPoint([]float64{rng.Float64() * 10, rng.Float64() * 10}, i)
	}

	if len(tree.leaves) != 100 {
		t.Errorf("expected 100 leaves, got %d", len(tree.leaves))
	}

	// Delete half
	for i := 0; i < 50; i++ {
		tree.forgetPoint(i)
	}

	if len(tree.leaves) != 50 {
		t.Errorf("expected 50 leaves after deletion, got %d", len(tree.leaves))
	}

	// Verify remaining leaves are accessible
	for i := 50; i < 100; i++ {
		if _, exists := tree.leaves[i]; !exists {
			t.Errorf("leaf %d should exist", i)
		}
		// Codisp should not panic
		_ = tree.codisp(i)
	}

	// Insert more
	for i := 100; i < 150; i++ {
		tree.insertPoint([]float64{rng.Float64() * 10, rng.Float64() * 10}, i)
	}

	if len(tree.leaves) != 100 {
		t.Errorf("expected 100 leaves, got %d", len(tree.leaves))
	}

	// Verify root count matches leaf count
	if tree.root.leafCount() != 100 {
		t.Errorf("root leaf count %d doesn't match actual leaves %d",
			tree.root.leafCount(), len(tree.leaves))
	}
}

// TestInsertPointCut tests the cut generation algorithm
func TestInsertPointCut(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tree := newRCTree(rng)
	tree.ndim = 2

	// Bbox: min=(0,0), max=(1,1)
	bbox := []float64{0.0, 0.0, 1.0, 1.0}
	point := []float64{2.0, 0.5} // Outside bbox in dimension 0

	// Run multiple cuts and verify they're valid
	for i := 0; i < 100; i++ {
		dim, val := tree.insertPointCut(point, bbox)

		if dim < 0 || dim >= tree.ndim {
			t.Errorf("cut dimension %d out of range", dim)
		}

		// Extended bbox should include the point
		minD := math.Min(bbox[dim], point[dim])
		maxD := math.Max(bbox[tree.ndim+dim], point[dim])

		if val < minD || val > maxD {
			t.Errorf("cut value %f outside extended bbox range [%f, %f]", val, minD, maxD)
		}
	}
}
