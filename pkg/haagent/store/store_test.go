package store

import (
	"io/ioutil"
	"os"
	"testing"
	"time"
)

// Test_StoreOpen tests that the store can be opened.
func Test_StoreOpen(t *testing.T) {
	s := New(false)
	tmpDir, _ := ioutil.TempDir("", "store_test")
	defer os.RemoveAll(tmpDir)

	s.RaftBind = "127.0.0.1:0"
	s.RaftDir = tmpDir
	if s == nil {
		t.Fatalf("failed to create store")
	}

	if err := s.Open(false, "node0"); err != nil {
		t.Fatalf("failed to open store: %s", err)
	}
}

// Test_StoreOpenSingleNode tests that a command can be applied to the log
func Test_StoreOpenSingleNode(t *testing.T) {
	s := New(false)
	tmpDir, _ := ioutil.TempDir("", "store_test")
	defer os.RemoveAll(tmpDir)

	s.RaftBind = "127.0.0.1:0"
	s.RaftDir = tmpDir
	if s == nil {
		t.Fatalf("failed to create store")
	}

	if err := s.Open(true, "node0"); err != nil {
		t.Fatalf("failed to open store: %s", err)
	}

	// Simple way to ensure there is a leader.
	time.Sleep(3 * time.Second)

	if err := s.Set("foo", "bar"); err != nil {
		t.Fatalf("failed to set key: %s", err.Error())
	}

	// Wait for committed log entry to be applied.
	time.Sleep(500 * time.Millisecond)
	value, err := s.Get("foo")
	if err != nil {
		t.Fatalf("failed to get key: %s", err.Error())
	}
	if value != "bar" {
		t.Fatalf("key has wrong value: %s", value)
	}

	if err := s.Delete("foo"); err != nil {
		t.Fatalf("failed to delete key: %s", err.Error())
	}

	// Wait for committed log entry to be applied.
	time.Sleep(500 * time.Millisecond)
	value, err = s.Get("foo")
	if err != nil {
		t.Fatalf("failed to get key: %s", err.Error())
	}
	if value != "" {
		t.Fatalf("key has wrong value: %s", value)
	}
}

// Test_StoreInMemOpenSingleNode tests that a command can be applied to the log
// stored in RAM.
func Test_StoreInMemOpenSingleNode(t *testing.T) {
	s := New(true)
	tmpDir, _ := ioutil.TempDir("", "store_test")
	defer os.RemoveAll(tmpDir)

	s.RaftBind = "127.0.0.1:0"
	s.RaftDir = tmpDir
	if s == nil {
		t.Fatalf("failed to create store")
	}

	if err := s.Open(true, "node0"); err != nil {
		t.Fatalf("failed to open store: %s", err)
	}

	// Simple way to ensure there is a leader.
	time.Sleep(3 * time.Second)

	if err := s.Set("foo", "bar"); err != nil {
		t.Fatalf("failed to set key: %s", err.Error())
	}

	// Wait for committed log entry to be applied.
	time.Sleep(500 * time.Millisecond)
	value, err := s.Get("foo")
	if err != nil {
		t.Fatalf("failed to get key: %s", err.Error())
	}
	if value != "bar" {
		t.Fatalf("key has wrong value: %s", value)
	}

	if err := s.Delete("foo"); err != nil {
		t.Fatalf("failed to delete key: %s", err.Error())
	}

	// Wait for committed log entry to be applied.
	time.Sleep(500 * time.Millisecond)
	value, err = s.Get("foo")
	if err != nil {
		t.Fatalf("failed to get key: %s", err.Error())
	}
	if value != "" {
		t.Fatalf("key has wrong value: %s", value)
	}
}
