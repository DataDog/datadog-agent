// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ringbuffer

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		capacity uint64
	}{
		{
			name:     "small capacity",
			capacity: 5,
		},
		{
			name:     "large capacity",
			capacity: 1000,
		},
		{
			name:     "single element",
			capacity: 1,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				rb := NewRingBuffer[int](tt.capacity)
				require.NotNil(
					t,
					rb,
				)
				assert.Equal(
					t,
					tt.capacity,
					rb.Capacity(),
				)
				assert.Equal(
					t,
					uint64(0),
					rb.Size(),
				)
				assert.Equal(
					t,
					tt.capacity,
					uint64(len(rb.buffer)),
				)
			},
		)
	}
}

func TestPushBasic(t *testing.T) {
	rb := NewRingBuffer[int](5)

	// Push single element
	rb.Push(1)
	assert.Equal(
		t,
		uint64(1),
		rb.Size(),
	)

	// Push multiple elements
	rb.Push(2)
	rb.Push(3)
	assert.Equal(
		t,
		uint64(3),
		rb.Size(),
	)
}

func TestPushUpToCapacity(t *testing.T) {
	capacity := uint64(5)
	rb := NewRingBuffer[int](capacity)

	// Fill up to capacity
	for i := uint64(0); i < capacity; i++ {
		rb.Push(int(i))
		assert.Equal(
			t,
			i+1,
			rb.Size(),
		)
	}

	assert.Equal(
		t,
		capacity,
		rb.Size(),
	)
}

func TestPushOverflow(t *testing.T) {
	capacity := uint64(3)
	rb := NewRingBuffer[int](capacity)

	// Fill to capacity
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	assert.Equal(
		t,
		capacity,
		rb.Size(),
	)

	// Overflow - should wrap around and overwrite oldest
	rb.Push(4)
	assert.Equal(
		t,
		capacity,
		rb.Size(),
		"size should remain at capacity after overflow",
	)

	rb.Push(5)
	rb.Push(6)
	assert.Equal(
		t,
		capacity,
		rb.Size(),
		"size should still be at capacity",
	)
}

func TestPushWithStrings(t *testing.T) {
	rb := NewRingBuffer[string](3)

	rb.Push("hello")
	rb.Push("world")
	rb.Push("test")

	assert.Equal(
		t,
		uint64(3),
		rb.Size(),
	)

	// Overflow
	rb.Push("overflow")
	assert.Equal(
		t,
		uint64(3),
		rb.Size(),
	)
}

func TestPushWithStructs(t *testing.T) {
	type TestStruct struct {
		ID   int
		Name string
	}

	rb := NewRingBuffer[TestStruct](2)

	rb.Push(
		TestStruct{
			ID:   1,
			Name: "first",
		},
	)
	assert.Equal(
		t,
		uint64(1),
		rb.Size(),
	)

	rb.Push(
		TestStruct{
			ID:   2,
			Name: "second",
		},
	)
	assert.Equal(
		t,
		uint64(2),
		rb.Size(),
	)

	// Overflow
	rb.Push(
		TestStruct{
			ID:   3,
			Name: "third",
		},
	)
	assert.Equal(
		t,
		uint64(2),
		rb.Size(),
	)
}

func TestPushSingleCapacity(t *testing.T) {
	rb := NewRingBuffer[int](1)

	rb.Push(1)
	assert.Equal(
		t,
		uint64(1),
		rb.Size(),
	)

	// Overflow immediately on second element
	rb.Push(2)
	assert.Equal(
		t,
		uint64(1),
		rb.Size(),
	)

	rb.Push(3)
	assert.Equal(
		t,
		uint64(1),
		rb.Size(),
	)
}

func TestCapacity(t *testing.T) {
	tests := []struct {
		name     string
		capacity uint64
	}{
		{
			"capacity 1",
			1,
		},
		{
			"capacity 10",
			10,
		},
		{
			"capacity 100",
			100,
		},
		{
			"capacity 1000",
			1000,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				rb := NewRingBuffer[int](tt.capacity)
				assert.Equal(
					t,
					tt.capacity,
					rb.Capacity(),
				)

				// Capacity should not change after pushes
				for i := uint64(0); i < tt.capacity*2; i++ {
					rb.Push(int(i))
				}
				assert.Equal(
					t,
					tt.capacity,
					rb.Capacity(),
				)
			},
		)
	}
}

func TestSize(t *testing.T) {
	rb := NewRingBuffer[int](5)

	// Initial size should be 0
	assert.Equal(
		t,
		uint64(0),
		rb.Size(),
	)

	// Size should increase with pushes
	for i := 1; i <= 5; i++ {
		rb.Push(i)
		assert.Equal(
			t,
			uint64(i),
			rb.Size(),
		)
	}

	// Size should stay at capacity after overflow
	rb.Push(6)
	assert.Equal(
		t,
		uint64(5),
		rb.Size(),
	)

	rb.Push(7)
	assert.Equal(
		t,
		uint64(5),
		rb.Size(),
	)
}

func TestReadAll(t *testing.T) {
	rb := NewRingBuffer[int](5)

	// Empty buffer should return empty slice with capacity size
	elements := rb.ReadAll()
	assert.Equal(
		t,
		5,
		len(elements),
	)

	// Push some elements
	rb.Push(10)
	rb.Push(20)
	rb.Push(30)

	elements = rb.ReadAll()
	assert.Equal(
		t,
		5,
		len(elements),
	)
	assert.Equal(
		t,
		10,
		elements[0],
	)
	assert.Equal(
		t,
		20,
		elements[1],
	)
	assert.Equal(
		t,
		30,
		elements[2],
	)
}

func TestReadAllAfterOverflow(t *testing.T) {
	rb := NewRingBuffer[int](3)

	// Fill and overflow
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	rb.Push(4) // Overwrites 1
	rb.Push(5) // Overwrites 2

	elements := rb.ReadAll()
	assert.Equal(
		t,
		3,
		len(elements),
	)

	// The buffer should contain [4, 5, 3] at indices [0, 1, 2]
	// but head is at 2, tail is at 2
	assert.Contains(
		t,
		elements,
		3,
	)
	assert.Contains(
		t,
		elements,
		4,
	)
	assert.Contains(
		t,
		elements,
		5,
	)
}

func TestReadAllDoesNotModifyBuffer(t *testing.T) {
	rb := NewRingBuffer[int](3)

	rb.Push(1)
	rb.Push(2)

	// Read multiple times
	elements1 := rb.ReadAll()
	elements2 := rb.ReadAll()

	// Modify returned slice should not affect buffer
	elements1[0] = 999

	assert.Equal(
		t,
		1,
		elements2[0],
	)
	assert.Equal(
		t,
		uint64(2),
		rb.Size(),
	)
}

func TestReadAllConcurrent(t *testing.T) {
	rb := NewRingBuffer[int](100)

	// Fill buffer
	for i := 0; i < 50; i++ {
		rb.Push(i)
	}

	var wg sync.WaitGroup
	numReaders := 10

	wg.Add(numReaders)
	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				elements := rb.ReadAll()
				assert.Equal(
					t,
					100,
					len(elements),
				)
			}
		}()
	}

	wg.Wait()
}

func TestConcurrentPush(t *testing.T) {
	capacity := uint64(1000)
	rb := NewRingBuffer[int](capacity)

	numGoroutines := 10
	pushesPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < pushesPerGoroutine; j++ {
				rb.Push(id*1000 + j)
			}
		}(i)
	}

	wg.Wait()

	// Should be at capacity or total pushed, whichever is smaller
	totalPushed := uint64(numGoroutines * pushesPerGoroutine)
	expectedSize := capacity
	if totalPushed < capacity {
		expectedSize = totalPushed
	}

	assert.Equal(
		t,
		expectedSize,
		rb.Size(),
	)
}

func TestConcurrentPushAndRead(t *testing.T) {
	capacity := uint64(100)
	rb := NewRingBuffer[int](capacity)

	var wg sync.WaitGroup

	// Start pushing goroutines
	numPushers := 5
	wg.Add(numPushers)
	for i := 0; i < numPushers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				rb.Push(id*100 + j)
			}
		}(i)
	}

	// Start reading goroutines
	numReaders := 3
	wg.Add(numReaders)
	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = rb.Size()
				_ = rb.Capacity()
				_ = rb.ReadAll()
			}
		}()
	}

	wg.Wait()

	// Verify final state
	assert.LessOrEqual(
		t,
		rb.Size(),
		rb.Capacity(),
	)
}

func TestInterfaceCompliance(t *testing.T) {
	var _ RingBufferInterface[int] = (*RingBuffer[int])(nil)

	rb := NewRingBuffer[int](5)
	var iface RingBufferInterface[int] = rb

	iface.Push(1)
	iface.Push(2)

	assert.Equal(
		t,
		uint64(2),
		iface.Size(),
	)
	assert.Equal(
		t,
		uint64(5),
		iface.Capacity(),
	)

	elements := iface.ReadAll()
	assert.Equal(
		t,
		5,
		len(elements),
	)
}

func TestGenericTypes(t *testing.T) {
	t.Run(
		"int",
		func(t *testing.T) {
			rb := NewRingBuffer[int](5)
			rb.Push(42)
			assert.Equal(
				t,
				uint64(1),
				rb.Size(),
			)
		},
	)

	t.Run(
		"string",
		func(t *testing.T) {
			rb := NewRingBuffer[string](5)
			rb.Push("test")
			assert.Equal(
				t,
				uint64(1),
				rb.Size(),
			)
		},
	)

	t.Run(
		"bool",
		func(t *testing.T) {
			rb := NewRingBuffer[bool](5)
			rb.Push(true)
			assert.Equal(
				t,
				uint64(1),
				rb.Size(),
			)
		},
	)

	t.Run(
		"pointer",
		func(t *testing.T) {
			rb := NewRingBuffer[*int](5)
			val := 42
			rb.Push(&val)
			assert.Equal(
				t,
				uint64(1),
				rb.Size(),
			)
		},
	)

	t.Run(
		"slice",
		func(t *testing.T) {
			rb := NewRingBuffer[[]byte](5)
			rb.Push([]byte("hello"))
			assert.Equal(
				t,
				uint64(1),
				rb.Size(),
			)
		},
	)
}

func TestWrapAround(t *testing.T) {
	capacity := uint64(3)
	rb := NewRingBuffer[int](capacity)

	// Fill the buffer
	for i := 0; i < int(capacity); i++ {
		rb.Push(i)
	}

	// Verify internal state after wrap
	assert.Equal(
		t,
		capacity,
		rb.Size(),
	)
	assert.Equal(
		t,
		uint64(0),
		rb.head,
	)
	assert.Equal(
		t,
		uint64(0),
		rb.tail,
	)

	// Add more to cause wrap
	rb.Push(100)
	assert.Equal(
		t,
		uint64(1),
		rb.head,
	)
	assert.Equal(
		t,
		uint64(1),
		rb.tail,
	)

	rb.Push(101)
	assert.Equal(
		t,
		uint64(2),
		rb.head,
	)
	assert.Equal(
		t,
		uint64(2),
		rb.tail,
	)

	rb.Push(102)
	assert.Equal(
		t,
		uint64(0),
		rb.head,
	)
	assert.Equal(
		t,
		uint64(0),
		rb.tail,
	)
}

func TestClearEmpty(t *testing.T) {
	rb := NewRingBuffer[int](5)

	// Clear empty buffer
	rb.Clear()

	assert.Equal(
		t,
		uint64(0),
		rb.Size(),
	)
	assert.Equal(
		t,
		uint64(0),
		rb.head,
	)
	assert.Equal(
		t,
		uint64(0),
		rb.tail,
	)
}

func TestClearPartiallyFilled(t *testing.T) {
	rb := NewRingBuffer[int](5)

	// Add some elements
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)

	assert.Equal(
		t,
		uint64(3),
		rb.Size(),
	)

	// Clear the buffer
	rb.Clear()

	assert.Equal(
		t,
		uint64(0),
		rb.Size(),
	)
	assert.Equal(
		t,
		uint64(0),
		rb.head,
	)
	assert.Equal(
		t,
		uint64(0),
		rb.tail,
	)

	// Verify buffer is actually cleared
	elements := rb.ReadAll()
	for _, elem := range elements {
		assert.Equal(
			t,
			0,
			elem,
		)
	}
}

func TestClearFullBuffer(t *testing.T) {
	capacity := uint64(3)
	rb := NewRingBuffer[int](capacity)

	// Fill buffer to capacity
	for i := 0; i < int(capacity); i++ {
		rb.Push(i + 1)
	}

	assert.Equal(
		t,
		capacity,
		rb.Size(),
	)

	// Clear the buffer
	rb.Clear()

	assert.Equal(
		t,
		uint64(0),
		rb.Size(),
	)
	assert.Equal(
		t,
		uint64(0),
		rb.head,
	)
	assert.Equal(
		t,
		uint64(0),
		rb.tail,
	)

	// Verify all elements are cleared
	elements := rb.ReadAll()
	for _, elem := range elements {
		assert.Equal(
			t,
			0,
			elem,
		)
	}
}

func TestClearAfterOverflow(t *testing.T) {
	capacity := uint64(3)
	rb := NewRingBuffer[int](capacity)

	// Fill and overflow
	for i := 0; i < 10; i++ {
		rb.Push(i)
	}

	assert.Equal(
		t,
		capacity,
		rb.Size(),
	)

	// Clear the buffer
	rb.Clear()

	assert.Equal(
		t,
		uint64(0),
		rb.Size(),
	)
	assert.Equal(
		t,
		uint64(0),
		rb.head,
	)
	assert.Equal(
		t,
		uint64(0),
		rb.tail,
	)
}

func TestClearAndReuse(t *testing.T) {
	rb := NewRingBuffer[int](5)

	// Add elements
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)

	// Clear
	rb.Clear()

	// Reuse buffer - should work normally
	rb.Push(10)
	rb.Push(20)

	assert.Equal(
		t,
		uint64(2),
		rb.Size(),
	)

	elements := rb.ReadAll()
	assert.Equal(
		t,
		10,
		elements[0],
	)
	assert.Equal(
		t,
		20,
		elements[1],
	)
}

func TestClearMultipleTimes(t *testing.T) {
	rb := NewRingBuffer[int](5)

	// Clear multiple times
	rb.Clear()
	rb.Clear()
	rb.Clear()

	assert.Equal(
		t,
		uint64(0),
		rb.Size(),
	)

	// Should still be usable
	rb.Push(1)
	assert.Equal(
		t,
		uint64(1),
		rb.Size(),
	)
}

func TestClearWithDifferentTypes(t *testing.T) {
	t.Run(
		"string",
		func(t *testing.T) {
			rb := NewRingBuffer[string](3)
			rb.Push("hello")
			rb.Push("world")

			rb.Clear()

			assert.Equal(
				t,
				uint64(0),
				rb.Size(),
			)

			elements := rb.ReadAll()
			for _, elem := range elements {
				assert.Equal(
					t,
					"",
					elem,
				)
			}
		},
	)

	t.Run(
		"struct",
		func(t *testing.T) {
			type TestStruct struct {
				ID   int
				Name string
			}

			rb := NewRingBuffer[TestStruct](3)
			rb.Push(
				TestStruct{
					ID:   1,
					Name: "test",
				},
			)

			rb.Clear()

			assert.Equal(
				t,
				uint64(0),
				rb.Size(),
			)

			elements := rb.ReadAll()
			for _, elem := range elements {
				assert.Equal(
					t,
					0,
					elem.ID,
				)
				assert.Equal(
					t,
					"",
					elem.Name,
				)
			}
		},
	)

	t.Run(
		"pointer",
		func(t *testing.T) {
			rb := NewRingBuffer[*int](3)
			val1 := 10
			val2 := 20
			rb.Push(&val1)
			rb.Push(&val2)

			rb.Clear()

			assert.Equal(
				t,
				uint64(0),
				rb.Size(),
			)

			elements := rb.ReadAll()
			for _, elem := range elements {
				assert.Nil(
					t,
					elem,
				)
			}
		},
	)
}

func TestClearConcurrent(t *testing.T) {
	rb := NewRingBuffer[int](1000)

	// Fill buffer
	for i := 0; i < 500; i++ {
		rb.Push(i)
	}

	var wg sync.WaitGroup

	// Concurrent clears
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			rb.Clear()
		}()
	}

	wg.Wait()

	// Buffer should be clear
	assert.Equal(
		t,
		uint64(0),
		rb.Size(),
	)
}

func TestClearConcurrentWithPush(t *testing.T) {
	rb := NewRingBuffer[int](100)

	var wg sync.WaitGroup

	// Concurrent pushes
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				rb.Push(id*100 + j)
			}
		}(i)
	}

	// Concurrent clears
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				rb.Clear()
			}
		}()
	}

	wg.Wait()

	// Size should be valid (0 or something <= capacity)
	assert.LessOrEqual(
		t,
		rb.Size(),
		rb.Capacity(),
	)
}

// Benchmark tests
func BenchmarkPush(b *testing.B) {
	rb := NewRingBuffer[int](1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Push(i)
	}
}

func BenchmarkPushSmallCapacity(b *testing.B) {
	rb := NewRingBuffer[int](10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Push(i)
	}
}

func BenchmarkPushLargeCapacity(b *testing.B) {
	rb := NewRingBuffer[int](10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Push(i)
	}
}

func BenchmarkConcurrentPush(b *testing.B) {
	rb := NewRingBuffer[int](1000)
	b.ResetTimer()
	b.RunParallel(
		func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				rb.Push(i)
				i++
			}
		},
	)
}

func BenchmarkSize(b *testing.B) {
	rb := NewRingBuffer[int](1000)
	for i := 0; i < 500; i++ {
		rb.Push(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rb.Size()
	}
}

func BenchmarkCapacity(b *testing.B) {
	rb := NewRingBuffer[int](1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rb.Capacity()
	}
}

func BenchmarkReadAll(b *testing.B) {
	rb := NewRingBuffer[int](1000)
	for i := 0; i < 500; i++ {
		rb.Push(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rb.ReadAll()
	}
}

func BenchmarkReadAllSmall(b *testing.B) {
	rb := NewRingBuffer[int](10)
	for i := 0; i < 5; i++ {
		rb.Push(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rb.ReadAll()
	}
}

func BenchmarkReadAllLarge(b *testing.B) {
	rb := NewRingBuffer[int](10000)
	for i := 0; i < 5000; i++ {
		rb.Push(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rb.ReadAll()
	}
}

func BenchmarkClear(b *testing.B) {
	rb := NewRingBuffer[int](1000)
	for i := 0; i < 500; i++ {
		rb.Push(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Clear()
		// Refill to keep benchmark consistent
		if i < b.N-1 {
			for j := 0; j < 500; j++ {
				rb.Push(j)
			}
		}
	}
}

func BenchmarkClearEmpty(b *testing.B) {
	rb := NewRingBuffer[int](1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Clear()
	}
}

func BenchmarkClearSmall(b *testing.B) {
	rb := NewRingBuffer[int](10)
	for i := 0; i < 5; i++ {
		rb.Push(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Clear()
		if i < b.N-1 {
			for j := 0; j < 5; j++ {
				rb.Push(j)
			}
		}
	}
}

func BenchmarkClearLarge(b *testing.B) {
	rb := NewRingBuffer[int](10000)
	for i := 0; i < 5000; i++ {
		rb.Push(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Clear()
		if i < b.N-1 {
			for j := 0; j < 5000; j++ {
				rb.Push(j)
			}
		}
	}
}

func BenchmarkClearAndPush(b *testing.B) {
	rb := NewRingBuffer[int](1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Clear()
		for j := 0; j < 100; j++ {
			rb.Push(j)
		}
	}
}

func BenchmarkConcurrentClear(b *testing.B) {
	rb := NewRingBuffer[int](1000)
	for i := 0; i < 500; i++ {
		rb.Push(i)
	}
	b.ResetTimer()
	b.RunParallel(
		func(pb *testing.PB) {
			for pb.Next() {
				rb.Clear()
			}
		},
	)
}
