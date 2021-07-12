// +build python,test

package python

import "testing"

func TestComponentTopology(t *testing.T) {
	testComponentTopology(t)
}

func TestRelationTopology(t *testing.T) {
	testRelationTopology(t)
}

func TestStartSnapshotCheck(t *testing.T) {
	testStartSnapshotCheck(t)
}

func TestStopSnapshotCheck(t *testing.T) {
	testStopSnapshotCheck(t)
}
