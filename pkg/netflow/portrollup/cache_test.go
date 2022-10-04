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

	cache := NewCache(4) // minutes
	cache.LastClean = MockTimeNow()

	// increment entry2 at t0
	cache.Increment("entry1")
	assert.Equal(t, uint8(1), cache.Get("entry1"))
	assert.Equal(t, uint8(4), cache.items["entry1"].ExpirationMinFromLastCheck)

	// increment entry2 at t1
	timeNow = func() time.Time {
		return MockTimeNow().Add(1 * time.Minute)
	}
	cache.Increment("entry2")
	assert.Equal(t, uint8(1), cache.Get("entry2"))
	assert.Equal(t, uint8(5), cache.items["entry2"].ExpirationMinFromLastCheck)

	// increment entry1 at t2
	timeNow = func() time.Time {
		return MockTimeNow().Add(2 * time.Minute)
	}
	cache.Increment("entry1")
	assert.Equal(t, uint8(2), cache.Get("entry1"))
	assert.Equal(t, uint8(6), cache.items["entry1"].ExpirationMinFromLastCheck)

	//// cleanup at t4
	//timeNow = func() time.Time {
	//	return MockTimeNow().Add(4 * time.Minute)
	//}
	//cache.Increment("entry1")
	//assert.Equal(t, uint8(2), cache.Get("entry1"))
	//assert.Equal(t, uint8(6), cache.items["entry1"].ExpirationMinFromLastCheck)

}

func TestPortCache_Increment(t *testing.T) {
	setMockTime()
	defer revertTime()

	cache := NewCache(4) // minutes
	cache.LastClean = MockTimeNow().Add(-2 * time.Minute)
	cache.Increment("entry1")
	cache.Increment("entry2")
	assert.Equal(t, uint8(1), cache.Get("entry1"))
	assert.Equal(t, uint8(1), cache.Get("entry2"))
	assert.Equal(t, uint8(6), cache.items["entry1"].ExpirationMinFromLastCheck)
	assert.Equal(t, uint8(6), cache.items["entry2"].ExpirationMinFromLastCheck)

	// increment entry1
	timeNow = func() time.Time {
		return MockTimeNow().Add(1 * time.Minute)
	}
	cache.Increment("entry1")
	assert.Equal(t, uint8(2), cache.Get("entry1"))
	assert.Equal(t, uint8(1), cache.Get("entry2"))
	assert.Equal(t, uint8(7), cache.items["entry1"].ExpirationMinFromLastCheck) // ExpirationMinFromLastCheck has been updated to 1 more min
	assert.Equal(t, uint8(6), cache.items["entry2"].ExpirationMinFromLastCheck)
}

func TestPortCache_getExpirationMinFromLastCheck_lastCheckLessThanDefaultExpiration(t *testing.T) {
	setMockTime()
	defer revertTime()

	cache := NewCache(4) // minutes
	cache.getExpirationMinFromLastCheck()
	cache.LastClean = MockTimeNow().Add(-2 * time.Minute)
	assert.Equal(t, uint8(6), cache.getExpirationMinFromLastCheck())
}

func TestPortCache_getExpirationMinFromLastCheck_lastCheckMoreThanDefaultExpiration(t *testing.T) {
	setMockTime()
	defer revertTime()

	cache := NewCache(4) // minutes
	cache.getExpirationMinFromLastCheck()
	cache.LastClean = MockTimeNow().Add(-5 * time.Minute)
	assert.Equal(t, uint8(9), cache.getExpirationMinFromLastCheck())
}

func TestPortCache_RefreshExpiration(t *testing.T) {
	setMockTime()
	defer revertTime()

	cache := NewCache(4) // minutes
	cache.LastClean = MockTimeNow()
	cache.Increment("entry1")
	assert.Equal(t, uint8(4), cache.items["entry1"].ExpirationMinFromLastCheck)

	timeNow = func() time.Time {
		return MockTimeNow().Add(1 * time.Minute)
	}
	cache.RefreshExpiration("entry1")
	assert.Equal(t, uint8(5), cache.items["entry1"].ExpirationMinFromLastCheck)
}

func TestPortCache_DeleteAllExpired(t *testing.T) {
	setMockTime()
	defer revertTime()

	cache := NewCache(4) // minutes
	cache.LastClean = MockTimeNow()
	cache.Increment("entry1")
	assert.Equal(t, uint8(4), cache.items["entry1"].ExpirationMinFromLastCheck)
	assert.Equal(t, 1, cache.ItemCount())

	timeNow = func() time.Time {
		return MockTimeNow().Add(3 * time.Minute)
	}
	cache.DeleteAllExpired()
	assert.Equal(t, 1, cache.ItemCount())

	timeNow = func() time.Time {
		return MockTimeNow().Add(4 * time.Minute)
	}
	cache.DeleteAllExpired()
	assert.Equal(t, 0, cache.ItemCount())
}
