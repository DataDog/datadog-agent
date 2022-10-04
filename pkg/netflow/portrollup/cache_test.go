package portrollup

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

var savedTimeNow = timeNow

// MockTimeNow mocks time.Now
var MockTimeNow = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2000-01-01 00:00:00"
	t, _ := time.Parse(layout, str)
	return t
}

func setMockTime() {
	timeNow = func() time.Time {
		return MockTimeNow()
	}
}

func revertTime() {
	timeNow = savedTimeNow
}

func TestPortCache_Scenario(t *testing.T) {
	setMockTime()
	defer revertTime()

	cache := NewCache(300)
	//cache.LastClean = MockTimeNow()

	t0 := timeNow()
	// increment entry2 at t0
	cache.Increment("entry1")
	assert.Equal(t, uint8(1), cache.Get("entry1"))
	assert.Equal(t, t0.Add(300*time.Second).UnixNano(), cache.items["entry1"].Expiration)

	// increment entry2 at t1
	timeNow = func() time.Time {
		return MockTimeNow().Add(1 * time.Minute)
	}
	t1 := timeNow()
	cache.Increment("entry2")
	assert.Equal(t, uint8(1), cache.Get("entry2"))
	assert.Equal(t, t1.Add(300*time.Second).UnixNano(), cache.items["entry2"].Expiration)
	assert.Equal(t, t0.Add(300*time.Second).UnixNano(), cache.items["entry1"].Expiration)

	// increment entry1 at t2
	timeNow = func() time.Time {
		return MockTimeNow().Add(2 * time.Minute)
	}
	t2 := timeNow()
	cache.Increment("entry1")
	assert.Equal(t, uint8(2), cache.Get("entry1"))
	assert.Equal(t, t2.Add(300*time.Second).UnixNano(), cache.items["entry1"].Expiration)

	// nothing to delete at t 5min
	timeNow = func() time.Time {
		return MockTimeNow().Add(5 * time.Minute)
	}
	cache.DeleteAllExpired()
	assert.Equal(t, 2, cache.ItemCount())

	// delete entry2 at t5
	timeNow = func() time.Time {
		return MockTimeNow().Add(6 * time.Minute)
	}
	cache.DeleteAllExpired()
	assert.Equal(t, 1, cache.ItemCount())
	assert.Equal(t, uint8(2), cache.Get("entry1"))
	assert.Equal(t, t2.Add(300*time.Second).UnixNano(), cache.items["entry1"].Expiration)
	assert.Equal(t, uint8(0), cache.Get("entry2"))

	// delete entry1 at t6
	timeNow = func() time.Time {
		return MockTimeNow().Add(7 * time.Minute)
	}
	cache.DeleteAllExpired()
	assert.Equal(t, 0, cache.ItemCount())
	assert.Equal(t, uint8(0), cache.Get("entry2"))

}

func TestPortCache_Increment(t *testing.T) {
	setMockTime()
	defer revertTime()

	cache := NewCache(300) // minutes
	cache.Increment("entry1")
	cache.Increment("entry2")
	assert.Equal(t, uint8(1), cache.Get("entry1"))
	assert.Equal(t, uint8(1), cache.Get("entry2"))
	//assert.Equal(t, uint8(6), cache.items["entry1"].Expiration)
	//assert.Equal(t, uint8(6), cache.items["entry2"].Expiration)
	t0 := timeNow()
	assert.Equal(t, t0.Add(300*time.Second).UnixNano(), cache.items["entry1"].Expiration)
	assert.Equal(t, t0.Add(300*time.Second).UnixNano(), cache.items["entry2"].Expiration)

	// increment entry1
	timeNow = func() time.Time {
		return MockTimeNow().Add(1 * time.Minute)
	}
	t1 := timeNow()
	cache.Increment("entry1")
	assert.Equal(t, uint8(2), cache.Get("entry1"))
	assert.Equal(t, uint8(1), cache.Get("entry2"))
	assert.Equal(t, t1.Add(300*time.Second).UnixNano(), cache.items["entry1"].Expiration)
	assert.Equal(t, t0.Add(300*time.Second).UnixNano(), cache.items["entry2"].Expiration)
}

func TestPortCache_RefreshExpiration(t *testing.T) {
	setMockTime()
	defer revertTime()

	cache := NewCache(300) // minutes
	cache.Increment("entry1")
	t0 := timeNow()
	//assert.Equal(t, uint8(4), cache.items["entry1"].Expiration)
	assert.Equal(t, t0.Add(300*time.Second).UnixNano(), cache.items["entry1"].Expiration)

	timeNow = func() time.Time {
		return MockTimeNow().Add(1 * time.Minute)
	}
	t1 := timeNow()
	cache.RefreshExpiration("entry1")
	//assert.Equal(t, uint8(5), cache.items["entry1"].Expiration)
	assert.Equal(t, t1.Add(300*time.Second).UnixNano(), cache.items["entry1"].Expiration)

}

func TestPortCache_DeleteAllExpired(t *testing.T) {
	setMockTime()
	defer revertTime()

	cache := NewCache(300) // minutes
	cache.Increment("entry1")
	t0 := timeNow()
	assert.Equal(t, t0.Add(300*time.Second).UnixNano(), cache.items["entry1"].Expiration)
	assert.Equal(t, 1, cache.ItemCount())

	timeNow = func() time.Time {
		return MockTimeNow().Add(3 * time.Minute)
	}
	cache.DeleteAllExpired()
	assert.Equal(t, 1, cache.ItemCount())

	timeNow = func() time.Time {
		return MockTimeNow().Add(5 * time.Minute)
	}
	cache.DeleteAllExpired()
	assert.Equal(t, 0, cache.ItemCount())
}
