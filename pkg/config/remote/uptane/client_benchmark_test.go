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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getBenchmarkTransactionalStore(b *testing.B) *transactionalStore {
	dir := b.TempDir()
	db, err := bbolt.Open(dir+"/remote-config.db", 0600, &bbolt.Options{})
	if err != nil {
		panic(err)
	}
	b.Cleanup(func() {
		db.Close()
	})
	return &transactionalStore{
		db:         db,
		cachedData: make(map[string]dbBucket),
	}
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
			cfg := newTestConfig(b, repository)
			ts := getBenchmarkTransactionalStore(b)
			client, err := newTestClient(ts, cfg)
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

// BenchmarkConcurrentRead exercises the exact call pattern used by the
// real gRPC handler (ClientGetConfigs) on every client poll: under
// concurrent load, fetch the current director Targets() and then fetch the
// content of every target path via TargetFiles(). This is the pattern the
// verifyUptane() read-state cache (cachedDirectorTargets/cachedTargetFiles)
// is meant to speed up, since without it every one of these reads would
// re-download and re-verify the same target files that were already
// verified moments earlier by Update().
func BenchmarkConcurrentRead(b *testing.B) {
	// transactionalStore logs at Debug level on every bucket operation
	// (Put/Get/Delete); left at the default level, that logging dominates
	// the allocation counts in this benchmark and masks the effect of the
	// read cache under measurement. Silence it for the duration of the run.
	log.SetupLogger(log.Default(), "off")

	for _, i := range []int{16, 64, 256} {
		b.Run(fmt.Sprintf("%d-configs", i), func(b *testing.B) {
			configTargets := data.TargetFiles{}
			directorTargets := data.TargetFiles{}
			targetFiles := []*pbgo.File{}
			target, meta := generateTarget()
			allPaths := make([]string, 0, i)
			for j := 0; j < i; j++ {
				targetPath := fmt.Sprintf("datadog/2/DEBUG/id/%d", j)
				configTargets[targetPath] = meta
				directorTargets[targetPath] = meta
				targetFiles = append(targetFiles, &pbgo.File{
					Path: targetPath,
					Raw:  target,
				})
				allPaths = append(allPaths, targetPath)
			}
			repository := newTestRepository(2, 1, configTargets, directorTargets, targetFiles)
			cfg := newTestConfig(b, repository)
			ts := getBenchmarkTransactionalStore(b)
			client, err := newTestClient(ts, cfg)
			if err != nil {
				b.Fatal(err)
			}

			// A single real Update() call: this verifies the repository via
			// verifyUptane(), which is exactly what populates the read
			// cache under test. All subsequent reads below are served from
			// that cache (as long as they land within the verify TTL).
			err = client.Update(repository.toUpdate())
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if _, err := client.Targets(); err != nil {
						b.Fatal(err)
					}
					if _, err := client.TargetFiles(allPaths); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
