// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tuf

import (
	"io/ioutil"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/stretchr/testify/assert"
	"github.com/theupdateframework/go-tuf/client"
)

func readMeta(remote *configRemoteStore, name string) ([]byte, error) {
	reader, _, err := remote.GetMeta(name)
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(reader)
}

func TestConfigRemoteStore(t *testing.T) {
	remote := &configRemoteStore{
		configMetas: pbgo.ConfigMetas{
			Roots: []*pbgo.TopMeta{
				{
					Version: 1,
					Raw:     []byte("test"),
				},
				{
					Version: 2,
					Raw:     []byte("test2"),
				},
			},
			Snapshot: &pbgo.TopMeta{
				Version: 3,
				Raw:     []byte("snapshot"),
			},
			Timestamp: &pbgo.TopMeta{
				Version: 4,
				Raw:     []byte("timestamp"),
			},
			TopTargets: &pbgo.TopMeta{
				Version: 5,
				Raw:     []byte("top-targets"),
			},
			DelegatedTargets: []*pbgo.DelegatedMeta{
				{
					Version: 6,
					Role:    "my-product",
					Raw:     []byte("my-config"),
				},
			},
		},
	}

	rootContent := getConfigRoot()
	content, err := readMeta(remote, "root.json")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, rootContent, content)

	content, err = readMeta(remote, "0.root.json")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, rootContent, content)

	content, err = readMeta(remote, "1.root.json")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, []byte("test"), content)

	content, err = readMeta(remote, "2.root.json")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, []byte("test2"), content)

	_, err = readMeta(remote, "100.root.json")
	if err == nil {
		t.Fatal("should not find 1.root.json")
	}

	// snapshot checks
	_, err = readMeta(remote, "snapshot.json")
	if err == nil {
		t.Fatal("should not find snapshot.json")
	}

	assert.ErrorAs(t, err, &client.ErrNotFound{})

	content, err = readMeta(remote, "3.snapshot.json")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, []byte("snapshot"), content)

	// for timestamp, version is ignored
	content, err = readMeta(remote, "5.timestamp.json")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, []byte("timestamp"), content)

	content, err = readMeta(remote, "timestamp.json")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, []byte("timestamp"), content)

	// targets checks
	_, err = readMeta(remote, "targets.json")
	if err == nil {
		t.Fatal("should not find targets.json")
	}

	assert.ErrorAs(t, err, &client.ErrNotFound{})

	_, err = readMeta(remote, "4.targets.json")
	if err == nil {
		t.Fatal("should not find 4.targets.json")
	}

	assert.ErrorAs(t, err, &client.ErrNotFound{})

	content, err = readMeta(remote, "5.targets.json")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, []byte("top-targets"), content)

	// delegated targets check
	_, err = readMeta(remote, "my-wrong-product.json")
	if err == nil {
		t.Fatal("should not find my-wrong-product.json")
	}

	_, err = readMeta(remote, "my-product.json")
	if err == nil {
		t.Fatal("should not find my-product.json")
	}

	_, err = readMeta(remote, "6.my-product.json")
	if err != nil {
		t.Fatal(err)
	}
}
