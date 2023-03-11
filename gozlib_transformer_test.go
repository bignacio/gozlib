package gozlib

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransformerCompressGZip(t *testing.T) {
	const bufferSize = 1024
	const originalLen = 5985

	output := bytes.NewBuffer([]byte{})

	compressor, err := NewGoGZipCompressor(output, CompressionLevelBestCompression, bufferSize)

	assert.NoError(t, err)

	original := makeTestData(originalLen)

	written, compError := io.Copy(compressor, bytes.NewBuffer(original))

	if assert.NoError(t, compError) {
		assert.NoError(t, Flush(compressor))

		assert.Greater(t, written, int64(0))

		// use the standard lib gzip code to decompress the data, acting as a verified validation component
		uncompressed, uncompError := stdLibGZipUncompress(output, originalLen)

		assert.NoError(t, uncompError)
		assert.Equal(t, original, uncompressed)
	}

	assert.NoError(t, compressor.Close())
}

func TestTransformerCompressGZipFlushOnClose(t *testing.T) {
	const bufferSize = 1024 * 8
	const originalLen = 1422

	output := bytes.NewBuffer([]byte{})

	compressor, _ := NewGoGZipCompressor(output, CompressionLevelBestCompression, bufferSize)

	original := makeTestData(originalLen)
	io.Copy(compressor, bytes.NewBuffer(original))
	assert.NoError(t, compressor.Close())

	uncompressed, _ := stdLibGZipUncompress(output, originalLen)

	assert.Equal(t, original, uncompressed)
}

func TestTransformerCompressEmptyInput(t *testing.T) {
	result := transformerCompressEmptyBuffer(t)

	// 20 for the gzip header
	assert.Equal(t, 20, result.Len())
}

func TestTransformerCompressUncompress(t *testing.T) {
	const originalLen = 1024

	verifyTransformerCompressUncompressBuffer(makeTestData(originalLen), t)
}

func TestTransformerCompressUncompressLowEntropyBuffer(t *testing.T) {
	const originalLen = 1024 * 7
	data := make([]byte, originalLen, originalLen)

	for i := range data {
		data[i] = 1
	}

	verifyTransformerCompressUncompressBuffer(data, t)
}

func TestTransformerCompressUncompressEmptyInput(t *testing.T) {
	input := transformerCompressEmptyBuffer(t)
	output := bytes.NewBuffer([]byte{})

	uncompressor, initErr := NewGoZLibUncompressor(input, 64)
	assert.NoError(t, initErr)
	uncompLen, uncompErr := io.Copy(output, uncompressor)
	assert.NoError(t, uncompErr)
	assert.NoError(t, uncompressor.Close())
	assert.Equal(t, int64(0), uncompLen)
	assert.Equal(t, 0, output.Len())
}

func TestTransformerUncompressGZip(t *testing.T) {
	const bufferSize = 1024
	const originalLen = 2000

	verifyTransformerUncompress(t, io.Copy, bufferSize, originalLen)
}

func TestTransformerUncompressGZipFixedSmallReadBuffer(t *testing.T) {
	const bufferSize = 1024
	const originalLen = 2000
	const readBufferSize = 32

	verifyTransformerUncompressFixedCopy(t, readBufferSize, bufferSize, originalLen)
}

func TestTransformerUncompressGZipFixedLargeReadBuffer(t *testing.T) {
	const bufferSize = 1024
	const originalLen = 5000
	const readBufferSize = originalLen * 10

	verifyTransformerUncompressFixedCopy(t, readBufferSize, bufferSize, originalLen)
}

func TestTransformerUncompressGZipFixedLargeReadAndWorkBuffers(t *testing.T) {
	const originalLen = 5000
	const readBufferSize = originalLen * 10
	const bufferSize = readBufferSize

	verifyTransformerUncompressFixedCopy(t, readBufferSize, bufferSize, originalLen)
}

func TestTransformerFailUncompressInvalidInput(t *testing.T) {
	input := makeTestData(1024)
	output := bytes.NewBuffer([]byte{})

	uncompressor, initErr := NewGoZLibUncompressor(bytes.NewBuffer(input), 64)
	assert.NoError(t, initErr)
	uncompLen, uncompErr := io.Copy(output, uncompressor)
	assert.ErrorIs(t, uncompErr, TransformerUncompressionError)
	assert.NoError(t, uncompressor.Close())
	assert.Equal(t, int64(0), uncompLen)
}

func TestTransformerUncompressEmptyInput(t *testing.T) {
	input := []byte{}
	output := bytes.NewBuffer([]byte{})

	uncompressor, initErr := NewGoZLibUncompressor(bytes.NewBuffer(input), 64)
	assert.NoError(t, initErr)
	uncompLen, uncompErr := io.Copy(output, uncompressor)
	assert.NoError(t, uncompErr)
	assert.NoError(t, uncompressor.Close())
	assert.Equal(t, int64(0), uncompLen)
}

type eofAwareReader struct {
	data *bytes.Buffer
}

func (r *eofAwareReader) Read(p []byte) (int, error) {

	rlen, err := r.data.Read(p)

	if rlen < len(p) {
		return rlen, io.EOF
	}

	if err != nil {
		return 0, err
	}

	return rlen, nil
}

func TestTransformerUncompressReadEOFWithData(t *testing.T) {
	const originalLen = 1122
	original := makeTestData(originalLen)
	compressed, compErr := stdLibGZipCompress(original)

	assert.NoError(t, compErr)

	eofReader := &eofAwareReader{data: compressed}
	uncompressor, uncompInitErr := NewGoZLibUncompressor(eofReader, 512)
	assert.NoError(t, uncompInitErr)

	uncompressed := bytes.NewBuffer([]byte{})
	uncompLen, uncompErr := io.Copy(uncompressed, uncompressor)

	assert.NoError(t, uncompErr)
	assert.Equal(t, int64(originalLen), uncompLen)
	assert.Equal(t, original, uncompressed.Bytes())
}

func TestTransformCanReuseResetCompressor(t *testing.T) {
	const bufferSize = 2048
	const originalLen = 5000
	const maxRuns = 1270
	original := makeTestData(originalLen)

	emptyInitial := bytes.NewBuffer([]byte{})
	compressor, err := NewGoGZipCompressor(emptyInitial, CompressionLevelBestCompression, bufferSize)
	defer compressor.Close()
	assert.NoError(t, err)

	// now that wwe have a compressor, let's use and reuse it a few times
	for runCount := 0; runCount < maxRuns; runCount++ {
		firstCompressed := bytes.NewBuffer([]byte{})
		ResetCompressor(firstCompressed, compressor)
		_, compError := io.Copy(compressor, bytes.NewBuffer(original))
		assert.NoError(t, Flush(compressor))
		assert.NoError(t, compError)

		secondCompressed := bytes.NewBuffer([]byte{})
		ResetCompressor(secondCompressed, compressor)
		_, compError = io.Copy(compressor, bytes.NewBuffer(original))
		assert.NoError(t, Flush(compressor))
		assert.NoError(t, compError)

		assert.Equal(t, firstCompressed, secondCompressed)
		uncompressed, uncompError := stdLibGZipUncompress(secondCompressed, originalLen)

		assert.NoError(t, uncompError)
		assert.Equal(t, original, uncompressed)
	}

}

func TestTransformCanReuseResetUncompressor(t *testing.T) {
	const bufferSize = 2048
	const originalLen = 5000
	const maxRuns = 103

	original := makeTestData(originalLen)

	compressed, err := stdLibGZipCompress(original)
	// keep the original compressed bytes since the byte buffer will be reset after each decompression run
	compressedBytes := compressed.Bytes()
	assert.NoError(t, err)

	uncompressor, initErr := NewGoZLibUncompressor(bytes.NewBuffer([]byte{}), 1024)
	defer uncompressor.Close()
	assert.NoError(t, initErr)

	// let's use and reuse the same uncompressor it a few times
	for runCount := 0; runCount < maxRuns; runCount++ {
		ResetUncompressor(bytes.NewBuffer(compressedBytes), uncompressor)
		uncompressed := bytes.NewBuffer([]byte{})
		_, uncompErr := io.Copy(uncompressed, uncompressor)
		assert.NoError(t, uncompErr)
		assert.Equal(t, uncompressed.Bytes(), original)
	}

}

func transformerCompressEmptyBuffer(t *testing.T) *bytes.Buffer {
	output := bytes.NewBuffer([]byte{})

	compressor, err := NewGoGZipCompressor(output, CompressionLevelBestCompression, 64)
	assert.NoError(t, err)
	io.Copy(compressor, bytes.NewBuffer([]byte{}))
	assert.NoError(t, compressor.Close())

	return output
}

func verifyTransformerCompressUncompressBuffer(data []byte, t *testing.T) {
	const bufferSize = 2048

	compressed := bytes.NewBuffer([]byte{})

	compressor, err := NewGoGZipCompressor(compressed, CompressionLevelBestCompression, bufferSize)
	assert.NoError(t, err)

	original := data
	_, compError := io.Copy(compressor, bytes.NewBuffer(original))

	assert.NoError(t, compressor.Close())
	assert.NoError(t, compError)

	uncompressor, initErr := NewGoZLibUncompressor(compressed, bufferSize)
	assert.NoError(t, initErr)
	uncompressed := bytes.NewBuffer([]byte{})

	uncompLen, uncompErr := io.Copy(uncompressed, uncompressor)
	assert.NoError(t, uncompErr)
	assert.NoError(t, uncompressor.Close())
	assert.Equal(t, int64(len(data)), uncompLen)
	assert.Equal(t, original, uncompressed.Bytes())
}

type copyDataFn func(dst io.Writer, src io.Reader) (written int64, err error)

func verifyTransformerUncompressFixedCopy(t *testing.T, readBufferSize int, workBufferSize uint32, originalLen uint32) {

	copyFn := func(dst io.Writer, src io.Reader) (written int64, err error) {

		buffer := make([]byte, readBufferSize, readBufferSize)
		totalRead := 0

		for {
			read, rerr := src.Read(buffer)
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				return 0, rerr
			}

			written, werr := dst.Write(buffer[0:read])

			if werr != nil {
				return 0, werr
			}

			if written != read {
				return 0, fmt.Errorf("written count should match read count")
			}

			totalRead = totalRead + read
		}

		return int64(totalRead), nil
	}

	verifyTransformerUncompress(t, copyFn, workBufferSize, originalLen)
}

func verifyTransformerUncompress(t *testing.T, copyFn copyDataFn, workBufferSize uint32, originalLen uint32) {
	original := makeTestData(originalLen)
	stdCompressed, stdCompErr := stdLibGZipCompress(original)

	if assert.NoError(t, stdCompErr) {
		output := bytes.NewBuffer([]byte{})

		uncompressor, initErr := NewGoZLibUncompressor(stdCompressed, workBufferSize)
		assert.NoError(t, initErr)
		uncompLen, uncompErr := copyFn(output, uncompressor)

		if assert.NoError(t, uncompErr) {
			assert.NoError(t, uncompressor.Close())
			assert.Equal(t, uncompLen, int64(originalLen))
			assert.Equal(t, original, output.Bytes())
		}
	}
}
