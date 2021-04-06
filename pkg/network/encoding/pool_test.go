package encoding

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPool(t *testing.T) {
	const numObjects = 10
	allocs := 0
	inUse := make([][]byte, numObjects)

	myPool := pool{
		New: func() interface{} {
			allocs++
			return make([]byte, 10)
		},
		TTL: time.Minute,
	}

	// called explicitly just for testing
	myPool.init()

	retrieveObjects := func() {
		for i := 0; i < numObjects; i++ {
			inUse[i] = myPool.Get().([]byte)
		}

		for i := 0; i < numObjects; i++ {
			myPool.Put(inUse[i])
			inUse[i] = nil
		}
	}

	now := time.Now()
	setTime(now, &myPool)
	retrieveObjects()
	assert.Equal(t, numObjects, allocs)

	// all calls to pool now should be served from pooled objects
	allocs = 0
	now = now.Add(30 * time.Second)
	setTime(now, &myPool)
	myPool.clear()
	retrieveObjects()
	assert.Equal(t, 0, allocs)

	// we're past the TTL now so all objects should have been freed (except by the sentinel value)
	now = now.Add(myPool.TTL + time.Second)
	setTime(now, &myPool)
	myPool.clear()
	assert.Equal(t, 1, myPool.list.Len())

	// objects should should be allocated again
	allocs = 0
	retrieveObjects()
	assert.Equal(t, numObjects, allocs)
}

func setTime(now time.Time, pool *pool) {
	pool.now = func() int64 {
		return now.Unix()
	}
}

func BenchmarkPoolSameObject(b *testing.B) {
	pool := pool{
		New: func() interface{} {
			return time.Now()
		},
	}

	var obj interface{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj = pool.Get()
		pool.Put(obj)
	}
}
