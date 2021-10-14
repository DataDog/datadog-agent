// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tuf

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/remote/store"
	"github.com/stretchr/testify/assert"
)

func TestLocalBoltStore(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "store")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	defer os.Remove(tmpFile.Name())

	store, err := store.NewStore(tmpFile.Name(), true, 2, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	localStore := &localBoltStore{
		name:  "director",
		store: store,
	}

	meta, err := localStore.GetMeta()
	if err != nil {
		t.Fatal(err)
	}

	rootMetadata := getDirectorRoot()

	assert.Equal(t, json.RawMessage(rootMetadata), meta["root.json"])

	if err := localStore.SetMeta("root.json", json.RawMessage("new_root")); err != nil {
		t.Error(err)
	}

	if err := localStore.SetMeta("test.json", json.RawMessage("test")); err != nil {
		t.Error(err)
	}

	meta, err = localStore.GetMeta()
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, json.RawMessage("new_root"), meta["root.json"])
	assert.Equal(t, json.RawMessage("test"), meta["test.json"])

	assert.Equal(t, len(meta), 2)
}
