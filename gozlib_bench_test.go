package gozlib

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	smallCompressedInputSizeBytes = 128
	largeCompressedInputSizeBytes = 1024 * 1024
)

var (
	smallTestData = makeTestData(uint32(smallCompressedInputSizeBytes))
	largeTestData = makeTestData(uint32(largeCompressedInputSizeBytes))
)

func BenchmarkGoGZipCompressSmall(b *testing.B) {
	benchmarkgoGZipCompress(b, smallTestData)
}

func BenchmarkStdLibGZipCompressSmall(b *testing.B) {
	benchmarkStdLibGZipCompress(b, smallTestData)
}

func BenchmarkGoGZipCompressLarge(b *testing.B) {
	benchmarkgoGZipCompress(b, largeTestData)
}

func BenchmarkStdLibGZipCompressLarge(b *testing.B) {
	benchmarkStdLibGZipCompress(b, largeTestData)
}

func benchmarkStdLibGZipCompress(b *testing.B, input []byte) {
	for i := 0; i < b.N; i++ {
		output := bytes.NewBuffer([]byte{})
		compressor, _ := gzip.NewWriterLevel(output, gzip.BestCompression)

		written, _ := io.Copy(compressor, bytes.NewBuffer(input))
		assert.Greater(b, written, int64(0))
		compressor.Close()
	}
}

func benchmarkgoGZipCompress(b *testing.B, input []byte) {
	const defaultBufferSize = 1024 * 8
	for i := 0; i < b.N; i++ {
		output := bytes.NewBuffer([]byte{})
		compressor, _ := NewGoGZipCompressor(output, CompressionLevelBestCompression, defaultBufferSize)
		written, _ := io.Copy(compressor, bytes.NewBuffer(input))

		assert.Greater(b, written, int64(0))
		compressor.Close()
	}
}
