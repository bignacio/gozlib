package gozlib

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGZipCompressStream(t *testing.T) {
	const originalLen = 1023 * 10
	const inputBufferSize = 1024
	const outputBufferSize = 1024

	verifyCompressStream(originalLen, inputBufferSize, outputBufferSize, t)
}

func TestGZipCompressStreamEmptyInput(t *testing.T) {
	const originalLen = 0
	const inputBufferSize = 1024
	const outputBufferSize = 1024

	verifyCompressStream(originalLen, inputBufferSize, outputBufferSize, t)
}

func TestGZipCompressStreamOutputBufferSmallerThanInput(t *testing.T) {
	const originalLen = 2048
	const inputBufferSize = 1024
	const outputBufferSize = 64

	verifyCompressStream(originalLen, inputBufferSize, outputBufferSize, t)
}

func TestGZipCompressStreamInputBufferSmallerThanOutput(t *testing.T) {
	const originalLen = 2048
	const inputBufferSize = 64
	const outputBufferSize = 1024

	verifyCompressStream(originalLen, inputBufferSize, outputBufferSize, t)
}

func TestGZipCompressStreamFailToWrite(t *testing.T) {

	inputReader := func(data []byte) uint32 {
		// it doesn't really matter what we read here
		return uint32(len(data))
	}

	outputWriter := func(data []byte) uint32 {
		// 0 is only returned on failure
		return 0
	}

	total, err := GoGZipCompressStream(CompressionLevelBestCompression, 100, 100, inputReader, outputWriter)

	assert.ErrorIs(t, err, StreamCompressError)
	assert.Equal(t, total, uint64(0))
}

func TestUncompressStream(t *testing.T) {
	const originalLen = 2048 * 7
	const inputBufferSize = 1024
	const outputBufferSize = 1024

	verifyUncompressStream(originalLen, inputBufferSize, outputBufferSize, t)
}

func TestUncompressStreamOutputBufferSmallerThanInput(t *testing.T) {
	const originalLen = 2048
	const inputBufferSize = 1024
	const outputBufferSize = 64

	verifyUncompressStream(originalLen, inputBufferSize, outputBufferSize, t)
}

func TestUncompressStreamInputBufferSmallerThanOutput(t *testing.T) {
	const originalLen = 2048
	const inputBufferSize = 64
	const outputBufferSize = 1024

	verifyUncompressStream(originalLen, inputBufferSize, outputBufferSize, t)
}

func TestUncompressStreamFailInvalidInput(t *testing.T) {
	const originalLen = 2048
	const inputBufferSize = 1024
	const outputBufferSize = 64

	inputReader := func(data []byte) uint32 {
		// generate invalid data
		for i := 0; i < len(data); i++ {
			data[i] = byte(i)
		}
		return uint32(len(data))
	}

	outputWriter := func(data []byte) uint32 {
		return uint32(len(data))
	}

	total, err := GoUncompressStream(100, 100, inputReader, outputWriter)

	assert.ErrorIs(t, err, StreamUncompressError)
	assert.Equal(t, uint64(0), total)
}

func TestCompressUncompressStream(t *testing.T) {
	const originalLen = 5729
	const inputBufferSize = 733
	const outputBufferSize = 691

	original := makeTestData(originalLen)
	uncompBuffer := bytes.NewBuffer(original)
	compressed := bytes.NewBuffer([]byte{})

	compInputReader := func(data []byte) uint32 {
		read, err := uncompBuffer.Read(data)

		if err != nil {
			return 0
		}
		return uint32(read)
	}

	compOutputWriter := func(data []byte) uint32 {
		written, err := compressed.Write(data)

		if err != nil {
			return 0
		}

		return uint32(written)
	}

	_, err := GoGZipCompressStream(CompressionLevelBestCompression, inputBufferSize, outputBufferSize, compInputReader, compOutputWriter)
	assert.NoError(t, err)

	uncompInputReader := func(data []byte) uint32 {
		read, err := compressed.Read(data)

		if err != nil {
			return 0
		}
		return uint32(read)
	}

	uncompressed := bytes.NewBuffer([]byte{})
	uncompOutputWriter := func(data []byte) uint32 {
		written, err := uncompressed.Write(data)

		if err != nil {
			return 0
		}

		return uint32(written)
	}

	uncompTotal, err := GoUncompressStream(inputBufferSize, outputBufferSize, uncompInputReader, uncompOutputWriter)
	assert.NoError(t, err)
	assert.Equal(t, uint64(originalLen), uncompTotal)
	assert.Equal(t, original, uncompressed.Bytes())
}

func verifyUncompressStream(originalLen uint32, inputBufferSize uint32, outputBufferSize uint32, t *testing.T) {
	original := makeTestData(originalLen)
	compressed, stdCompErr := stdLibGZipCompress(original)
	assert.NoError(t, stdCompErr)

	inputReader := func(data []byte) uint32 {
		read, err := compressed.Read(data)

		if err != nil {
			return 0
		}
		return uint32(read)
	}

	uncompressed := bytes.NewBuffer([]byte{})
	outputWriter := func(data []byte) uint32 {
		written, err := uncompressed.Write(data)

		if err != nil {
			return 0
		}

		return uint32(written)
	}

	total, err := GoUncompressStream(inputBufferSize, outputBufferSize, inputReader, outputWriter)

	assert.NoError(t, err)
	assert.Equal(t, uint64(originalLen), total)
	assert.Equal(t, original, uncompressed.Bytes())
}

func verifyCompressStream(originalLen uint32, inputBufferSize uint32, outputBufferSize uint32, t *testing.T) {
	original := makeTestData(originalLen)
	uncompBuffer := bytes.NewBuffer(original)
	compressed := bytes.NewBuffer([]byte{})

	inputReader := func(data []byte) uint32 {
		read, err := uncompBuffer.Read(data)

		if err != nil {
			return 0
		}
		return uint32(read)
	}

	outputWriter := func(data []byte) uint32 {
		written, err := compressed.Write(data)

		if err != nil {
			return 0
		}

		return uint32(written)
	}

	total, err := GoGZipCompressStream(CompressionLevelBestCompression, inputBufferSize, outputBufferSize, inputReader, outputWriter)

	assert.NoError(t, err)
	assert.Greater(t, total, uint64(0))

	stdUncompressed, uncompErr := stdLibGZipUncompress(compressed, int64(originalLen))

	assert.NoError(t, uncompErr)
	assert.Equal(t, stdUncompressed, original)
}
