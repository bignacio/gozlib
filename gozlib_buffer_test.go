package gozlib

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGoGZipCompressBuffer(t *testing.T) {
	const inputSize = 3712
	const outputSize = inputSize + 64 // enough output buffer to keep the compressed data + metadata
	input := makeTestData(inputSize)
	output := make([]byte, 0, outputSize)

	compLen, err := GoGZipCompressBuffer(CompressionLevelBestSpeed, input, output)

	assert.NoError(t, err)
	assert.Greater(t, compLen, uint64(0))

	compressedOut := output[:compLen]
	stdUncompressed, uncompErr := stdLibGZipUncompress(bytes.NewBuffer(compressedOut), int64(inputSize))

	assert.NoError(t, uncompErr)
	assert.Equal(t, stdUncompressed, input)
}

func TestGoGZipCompressBufferFailOutputSizeTooSmall(t *testing.T) {
	verifyGoGZipCompressBufferInvalidOutput(64, BufferCompressError, t)
}

func TestGoGZipCompressBufferFailOutputNotPreallocated(t *testing.T) {
	verifyGoGZipCompressBufferInvalidOutput(0, OutputBufferTooSmallError, t)

}

func verifyGoGZipCompressBufferInvalidOutput(outputSize uint32, expectedErr error, t *testing.T) {
	const inputSize = 3712
	input := makeTestData(inputSize)
	output := make([]byte, 0, outputSize)

	compLen, err := GoGZipCompressBuffer(CompressionLevelBestSpeed, input, output)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, uint64(0), compLen)
}

func TestGoUncompressBuffer(t *testing.T) {
	const originalSize = 3712
	const outputSize = originalSize
	original := makeTestData(originalSize)

	uncompressed := make([]byte, 0, outputSize)
	compressed, stdCompErr := stdLibGZipCompressSlice(original)

	assert.NoError(t, stdCompErr)

	uncompLen, uncompErr := GoUncompressBuffer(compressed, uncompressed)

	assert.NoError(t, uncompErr)
	assert.Equal(t, uint64(originalSize), uncompLen)
	assert.Equal(t, original, uncompressed[:uncompLen])
}

func TestGoUncompressBufferFailInvalidInput(t *testing.T) {
	const inputSize = 4111
	const outputSize = inputSize + 100
	invalidInput := makeTestData(inputSize)
	uncompressed := make([]byte, 0, outputSize)

	uncompLen, uncompErr := GoUncompressBuffer(invalidInput, uncompressed)
	assert.Equal(t, uint64(0), uncompLen)
	assert.ErrorIs(t, uncompErr, BufferUncompressError)
}

func TestGoUncompressBufferFailOutputSizeTooSmall(t *testing.T) {
	const originalSize = 3712
	verifyBufferUncompressBufferInvalidOutput(32, BufferUncompressError, t)
}

func TestGoUncompressBufferFailOutputNotPreallocated(t *testing.T) {
	verifyBufferUncompressBufferInvalidOutput(0, OutputBufferTooSmallError, t)
}

func verifyBufferUncompressBufferInvalidOutput(outputSize uint32, expectedErr error, t *testing.T) {
	const originalSize = 4111
	original := makeTestData(originalSize)

	uncompressed := make([]byte, outputSize, outputSize)
	compressed, stdCompErr := stdLibGZipCompressSlice(original)

	assert.NoError(t, stdCompErr)

	uncompLen, uncompErr := GoUncompressBuffer(compressed, uncompressed)
	assert.Equal(t, uint64(0), uncompLen)
	assert.ErrorIs(t, uncompErr, expectedErr)

}

func TestGoCompressUncompressBuffer(t *testing.T) {
	const inputSize = 3712
	const outputSize = inputSize + 64
	input := makeTestData(inputSize)
	output := make([]byte, 0, outputSize)

	compLen, compError := GoGZipCompressBuffer(CompressionLevelBestSpeed, input, output)
	assert.NoError(t, compError)

	compressed := output[:compLen]
	uncompressed := make([]byte, 0, inputSize)

	uncompLen, uncompErr := GoUncompressBuffer(compressed, uncompressed)

	assert.NoError(t, uncompErr)
	assert.Equal(t, uint64(inputSize), uncompLen)
	assert.Equal(t, input, uncompressed[:uncompLen])
}
