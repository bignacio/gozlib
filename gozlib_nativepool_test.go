package gozlib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllocAndReturnVariousSize(t *testing.T) {
	pool := NewNativeSlicePool()
	defer pool.Free()

	const maxBits = 20 // 1 << 20 == 1M max
	for bitCount := 0; bitCount <= maxBits; bitCount++ {
		size := 1 << bitCount
		data := pool.Acquire(size)
		assert.Equal(t, 0, len(data))
		assert.Equal(t, size, cap(data))
		pool.Return(data)
	}
}

func TestNativePoolAllocAndReuse(t *testing.T) {
	const desiredBufferSize = 1024
	pool := NewNativeSlicePool()
	defer pool.Free()

	data := pool.Acquire(desiredBufferSize)
	assert.Equal(t, 0, len(data))
	assert.Equal(t, desiredBufferSize, cap(data))

	tag := []byte{'a', 'b', 'c', '1', '2', '3'}
	// the returned slice should be reused and unmodified
	// in a real application, anything acquired from the pool should be considered unitialized memory
	data = data[:len(tag)]
	copy(data, tag)

	pool.Return(data)

	dataAfterReturned := pool.Acquire(desiredBufferSize)
	actual := dataAfterReturned[:len(tag)]
	assert.Equal(t, tag, actual)
}
