//go:build linux
// +build linux

package api

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"syscall"
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

func generateHTTPRequest(pid int32) *http.Request {
	ctx := context.WithValue(context.Background(), ucredKey{}, &syscall.Ucred{Pid: pid})
	r, err := http.NewRequestWithContext(ctx, "GET", "/", nil)
	if err != nil {
		log.Fatal(err)
	}
	return r
}

func TestContainerID(t *testing.T) {
	t.Run("expiry", func(t *testing.T) {
		// Insert a cached ID
		cv := &cacheVal{containerID: "test-cid"}
		cv.accessed.Store(time.Now())
		cache.cache[0] = cv

		// Make sure the cached ID is retrieved
		cid, ok := cache.ContainerID(0)
		assert.True(t, ok)
		assert.Equal(t, "test-cid", cid)

		// Push the cached ID out of the cache and make sure it is not retrieved
		cv.accessed.Store(time.Now().Add(-10 * time.Minute))
		cid, ok = cache.ContainerID(0)
		assert.False(t, ok)
		assert.Equal(t, "", cid)
	})
	t.Run("read", func(t *testing.T) {
		// Read cgroup Id from test file
		p := createCgroupFile(cgroup)
		createPath = func(pid int32) string {
			return p
		}
		r := generateHTTPRequest(0)
		cid := getContainerID(r)
		assert.Equal(t, "b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361", cid)

		// This will cause the read to return a different container ID
		p = createCgroupFile(cgroup2)
		createPath = func(pid int32) string {
			return p
		}
		// Retrieve ID should still return the old ID since it is still valid in the cache
		cid = getContainerID(r)
		assert.Equal(t, "b1a26054402a9c3786c2ae8a48cc54b0b1dfd7d999f36159b47865bbf976c361", cid)

		// Push the cached ID out of the cache and make sure we read the new ID.
		cv := cache.cache[0]
		cv.accessed.Store(time.Now().Add(-10 * time.Minute))
		cid = getContainerID(r)
		assert.Equal(t, "861fffad796a2f1d8e7bc3243b4a5fe36dc8a7a277fa5c5d6b252eb60f8cc258", cid)
	})
}

func BenchmarkCacheReadParallel(b *testing.B) {
	p := createCgroupFile(cgroup)
	createPath = func(pid int32) string {
		return p
	}
	r := generateHTTPRequest(0)
	n := b.N / 3
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				getContainerID(r)
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
	r := generateHTTPRequest(0)
	for i := 0; i < b.N; i++ {
		getContainerID(r)
	}
}

func BenchmarkCacheReadFull(b *testing.B) {
	p := createCgroupFile(cgroup)
	createPath = func(pid int32) string {
		return p
	}
	for i := 0; i < 100000; i++ {
		cv := &cacheVal{containerID: "test-cid"}
		cv.accessed.Store(time.Now())
		cach.cache[int32(i)] = cv
	}
	r := generateHTTPRequest(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.WithValue(context.Background(), ucredKey{}, &syscall.Ucred{Pid: rand.Int31n(100000)})
		r.WithContext(ctx)
		getContainerID(r)
	}
}

func BenchmarkCacheReadContention(b *testing.B) {
	p := createCgroupFile(cgroup)
	createPath = func(pid int32) string {
		return p
	}

	go func() {
		r := generateHTTPRequest(0)
		fmt.Printf("Populating Container IDs\n")
		for i := 100000; i < 200000; i++ {
			ctx := context.WithValue(context.Background(), ucredKey{}, &syscall.Ucred{Pid: int32(i)})
			r.WithContext(ctx)
			getContainerID(r)
		}
	}()

	//fmt.Printf("Retrieving Container IDs\n")
	r := generateHTTPRequest(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.WithValue(context.Background(), ucredKey{}, &syscall.Ucred{Pid: rand.Int31n(100000)})
		r.WithContext(ctx)
		getContainerID(r)
	}
}
