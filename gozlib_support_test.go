package gozlib

import (
	"bytes"
	"compress/gzip"
	"math/rand"

	"io"
)

func makeTestData(len uint32) []byte {
	data := make([]byte, len)

	for i := uint32(0); i < len; i++ {
		data[i] = byte(rand.Int() % 128)
	}

	return data
}

func stdLibGZipUncompress(compressed *bytes.Buffer, size int64) ([]byte, error) {
	decompressed := bytes.NewBuffer(make([]byte, size, size))
	decompressed.Reset()
	reader, err := gzip.NewReader(compressed)
	if err != nil {
		return nil, err
	}

	_, werr := io.Copy(decompressed, reader)

	return decompressed.Bytes(), werr
}

func stdLibGZipCompressSlice(data []byte) ([]byte, error) {
	compressed := &bytes.Buffer{}
	writer, gerr := gzip.NewWriterLevel(compressed, gzip.BestCompression)
	if gerr != nil {
		return nil, gerr
	}

	_, err := io.Copy(writer, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	err = writer.Close()

	return compressed.Bytes(), err
}

func stdLibGZipCompress(data []byte) (*bytes.Buffer, error) {
	compressed, err := stdLibGZipCompressSlice(data)

	if err != nil {
		return nil, err
	}

	return bytes.NewBuffer(compressed), nil
}
