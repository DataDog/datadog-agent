// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"fmt"
	"testing"

	"github.com/DataDog/go-tuf/data"
	"go.etcd.io/bbolt"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func getBenchmarkDB(b *testing.B) *bbolt.DB {
	dir := b.TempDir()
	db, err := bbolt.Open(dir+"/remote-config.db", 0600, &bbolt.Options{})
	if err != nil {
		panic(err)
	}
	b.Cleanup(func() {
		db.Close()
	})
	return db
}

func BenchmarkVerify(b *testing.B) {
	for i := 1; i <= 128; i *= 2 {
		b.Run(fmt.Sprintf("verify-%d-configs", i), func(b *testing.B) {
			configTargets := data.TargetFiles{}
			directorTargets := data.TargetFiles{}
			targetFiles := []*pbgo.File{}
			target, meta := generateTarget()
			for j := 0; j < i; j++ {
				targetPath := fmt.Sprintf("datadog/2/DEBUG/id/%d", j)
				configTargets[targetPath] = meta
				directorTargets[targetPath] = meta
				targetFiles = append(targetFiles, &pbgo.File{
					Path: targetPath,
					Raw:  target,
				})
			}
			repository := newTestRepository(2, 1, configTargets, directorTargets, targetFiles)
			cfg := newTestConfig(repository)
			db := getBenchmarkDB(b)
			client, err := newTestClient(db, cfg)
			if err != nil {
				b.Fatal(err)
			}
			err = client.Update(repository.toUpdate())
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for j := 0; j < b.N; j++ {
				client.cachedVerify = false
				err = client.verify()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
