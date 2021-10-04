// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"io/ioutil"
	"math/rand"
	"os"
	"sort"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/stretchr/testify/assert"
)

func newTestConfig(version uint64) *pbgo.ConfigResponse {
	return &pbgo.ConfigResponse{
		TargetFiles: []*pbgo.File{{
			Path: "config",
			Raw:  []byte("my-config"),
		}},
		DirectoryTargets: &pbgo.TopMeta{
			Version: version,
		},
		ConfigSnapshotVersion: version,
	}
}

func TestStore(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "store")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	defer os.Remove(tmpFile.Name())

	store, err := NewStore(tmpFile.Name(), true, 2, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	testConfig := newTestConfig(1)
	testConfig2 := newTestConfig(2)

	t.Run("store-configs", func(t *testing.T) {
		if err := store.StoreConfig(pbgo.Product_APPSEC.String(), testConfig); err != nil {
			t.Fatal(err)
		}

		if err := store.StoreConfig(pbgo.Product_APPSEC.String(), testConfig2); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("get-configs", func(t *testing.T) {
		configs, err := store.GetConfigs(pbgo.Product_APPSEC.String())
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, len(configs), 2)

		assert.ObjectsAreEqual(testConfig, configs[0])
		assert.ObjectsAreEqual(testConfig, configs[1])
	})

	t.Run("prune-configs", func(t *testing.T) {
		testConfig2 := newTestConfig(2)
		if err := store.StoreConfig(pbgo.Product_APPSEC.String(), testConfig2); err != nil {
			t.Fatal(err)
		}

		testConfig3 := newTestConfig(3)
		if err := store.StoreConfig(pbgo.Product_APPSEC.String(), testConfig3); err != nil {
			t.Fatal(err)
		}

		testConfig4 := newTestConfig(4)
		if err := store.StoreConfig(pbgo.Product_APPSEC.String(), testConfig4); err != nil {
			t.Fatal(err)
		}

		if err := store.StoreConfig(pbgo.Product_APPSEC.String(), testConfig4); err != nil {
			t.Fatal(err)
		}

		configs, err := store.GetConfigs(pbgo.Product_APPSEC.String())
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, 2, len(configs))

		assert.ObjectsAreEqual(testConfig3, configs[0])
		assert.ObjectsAreEqual(testConfig4, configs[1])
	})

	t.Run("sort", func(t *testing.T) {
		var testConfigs []*pbgo.ConfigResponse
		for i := uint64(1); i <= 100; i++ {
			testConfigs = append(testConfigs, newTestConfig(i))
		}

		sort.Slice(testConfigs, func(i, j int) bool {
			return rand.Intn(2) == 0
		})

		for _, testConfig := range testConfigs {
			if err := store.StoreConfig(pbgo.Product_APPSEC.String(), testConfig); err != nil {
				t.Fatal(err)
			}
		}
		configs, err := store.GetConfigs(pbgo.Product_APPSEC.String())
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, 2, len(configs))

		assert.Equal(t, uint64(99), configs[0].ConfigSnapshotVersion)
		assert.Equal(t, uint64(100), configs[1].ConfigSnapshotVersion)
	})
}
