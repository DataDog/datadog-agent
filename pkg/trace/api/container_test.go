//go:build linux
// +build linux

package api

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var cgroup string = `
15:name=elogind:/
14:name=systemd:/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
13:misc:/
12:pids:/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
11:hugetlb:/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
10:net_prio:/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
9:perf_event:/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
8:net_cls:/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
7:freezer:/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
6:devices:/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
5:memory:/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
4:blkio:/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
3:cpuacct:/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
2:cpu:/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
1:cpuset:/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
0::/docker/b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361
`

var cgroup2 string = `
15:name=elogind:/
14:name=systemd:/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
13:misc:/
12:pids:/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
11:hugetlb:/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
10:net_prio:/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
9:perf_event:/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
8:net_cls:/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
7:freezer:/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
6:devices:/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
5:memory:/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
4:blkio:/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
3:cpuacct:/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
2:cpu:/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
1:cpuset:/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
0::/docker/861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258
`

func createCgroupFile(val string) string {
	f, err := ioutil.TempFile("", "agent-test-cgroup")
	if err != nil {
		log.Fatal(err)
	}
	io.WriteString(f, val)
	defer f.Close()
	return f.Name()
}

func TestContainerID(t *testing.T) {
	t.Run("expiry", func(t *testing.T) {
		// Insert a cached ID
		cv := &cacheVal{containerID: "test-cid"}
		cv.accessed.Store(time.Now())
		containerCache[0] = cv

		// Make sure the cached ID is retrieved
		cid, ok := cachedContainerID(0)
		assert.True(t, ok)
		assert.Equal(t, "test-cid", cid)

		// Push the cached ID out of the cache and make sure it is not retrieved
		cv.accessed.Store(time.Now().Add(-10 * time.Minute))
		cid, ok = cachedContainerID(0)
		assert.False(t, ok)
		assert.Equal(t, "", cid)
	})
	t.Run("read", func(t *testing.T) {
		// Read cgroup Id from test file
		p := createCgroupFile(cgroup)
		createPath = func(pid int32) string {
			return p
		}
		cid := retrieveContainerID(0)
		assert.Equal(t, "b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361", cid)

		// This will cause the read to return a different container ID
		p = createCgroupFile(cgroup2)
		createPath = func(pid int32) string {
			return p
		}
		// Retrieve ID should still return the old ID since it is still valid in the cache
		cid = retrieveContainerID(0)
		assert.Equal(t, "b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361", cid)

		// Push the cached ID out of the cache and make sure we read the new ID.
		cv := containerCache[0]
		cv.accessed.Store(time.Now().Add(-10 * time.Minute))
		cid = retrieveContainerID(0)
		assert.Equal(t, "861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258", cid)
	})
}

func BenchmarkCacheReadParallel(b *testing.B) {
	p := createCgroupFile(cgroup)
	createPath = func(pid int32) string {
		return p
	}
	n := b.N / 3
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				retrieveContainerID(0)
			}
		}()
	}
	wg.Wait()
}

func BenchmarkCacheRead(b *testing.B) {
	p := createCgroupFile(cgroup)
	createPath = func(pid int32) string {
		return p
	}
	for i := 0; i < b.N; i++ {
		retrieveContainerID(0)
	}
}

func BenchmarkCacheReadFull(b *testing.B) {
	p := createCgroupFile(cgroup)
	createPath = func(pid int32) string {
		return p
	}
	//fmt.Printf("Populating Container IDs\n")
	for i := 0; i < 100000; i++ {
		//retrieveContainerID(int32(i))
		cv := &cacheVal{containerID: "test-cid"}
		cv.accessed.Store(time.Now())
		containerCache[int32(i)] = cv
	}
	//fmt.Printf("Retrieving Container IDs\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		retrieveContainerID(rand.Int31n(100000))
	}
}

func BenchmarkCacheReadContention(b *testing.B) {
	p := createCgroupFile(cgroup)
	createPath = func(pid int32) string {
		return p
	}

	go func() {
		fmt.Printf("Populating Container IDs\n")
		for i := 100000; i < 200000; i++ {
			retrieveContainerID(int32(i))
		}
	}()

	//fmt.Printf("Retrieving Container IDs\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		retrieveContainerID(rand.Int31n(100000))
	}
}
