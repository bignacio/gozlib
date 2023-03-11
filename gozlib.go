// GoZLib is a wrapper for zlib, using cgo for interoperability with the zlib library
// See https://github.com/madler/zlib for details about zlib

// Using this package requires cgo and a gnu compiler (clang or gcc), as well as the development version of zlib installed
// By default, it expect the zlib header and so files to be in the standard include and library path. If not, you can override it
// by setting the appropriate paths in the environment variables CGO_CFLAGS and CGO_LDFLAGS
// Internally gozlib utilizes an off-heap memory pool to maximize memory usage. At this moment, allocated memory is never
// returned to the system so gozlib is best used when gzip operations are frequent and constant.
// This pool is also available for use in the Go code as a way to allocate and reuse byte slices.
// See NativeSlicePool for details
package gozlib

/*
#cgo CFLAGS: -Wall -Wno-unused-variable -Werror -O3 -g0 -DGOZLIB_GO_INTEROP -DNDEBUG
#cgo LDFLAGS: -lz
#include "zwrapper/gozlib.h"
#include "zwrapper/gozlib.c"
*/
import "C"
import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"unsafe"
)

/*
to compile tests with memory sanitization enabled
export MSAN_OPTIONS=verbosity=1:exitcode=1:print_stats=1
CC=clang go test -c -msan .

also run tests with GODEBUG=cgocheck=2
for example
GODEBUG=cgocheck=2 go test -a ./... -count=1 -gcflags=all=-d=checkptr
for information pertaining to memory usage between Go and C, see also see https://github.com/golang/go/issues/19135
*/

type CompressionLevel int
type TransformMode int

const (
	CompressionLevelBestCompression CompressionLevel = C.Z_BEST_COMPRESSION
	CompressionLevelBestSpeed       CompressionLevel = C.Z_BEST_SPEED
)

const (
	TransformModeZLib       TransformMode = 0
	TransformModeGZip       TransformMode = 1
	TransformModeUncompress TransformMode = 2
)

const (
	wrapErrorFormat = "%w ZLib error code %d"
)

var (
	// transformer
	TransformerUncompressionError  = errors.New("error uncompressing data")
	TransformerInitializationError = errors.New("error initializing transformer")
	TransformerCompressionError    = errors.New("error compressing data")

	// streaming
	StreamCompressError   = errors.New("error streaming compressed data")
	StreamUncompressError = errors.New("error streaming uncompressed data")

	// buffer to buffer
	OutputBufferTooSmallError = errors.New("output buffer too small")
	BufferCompressError       = errors.New("error compressing buffer")
	BufferUncompressError     = errors.New("error uncompressing buffer")
)

type transformerWriterHandler struct {
	writtenBytes     int
	eventHandlers    *streamEventHandlers
	eventHandlersPtr unsafe.Pointer
}

// goZLibTransformer provides supports the implementation of compression and uncompression
// behaviour
type goZLibTransformer struct {
	input       io.Reader
	output      io.Writer
	transformer *C.GoZLibTransformer
	twh         *transformerWriterHandler
}

type goGZipCompressor struct {
	goZLibTransformer
}

// NewGoGZipCompressor creates a new gzip compressor
// The compressor writes compressed data to the provided output Writer.
// The level parameter specifies the compression level. It can be set to CompressionLevelBestCompression or CompressionLevelBestSpeed
// The bufferSize parameter specifies the size of the buffer used by the compressor. For best performance, set it to a size that's power 2,
// large enough for the expected input.
// Returns an io.WriteCloser for writing compressed data and an error, if any.
func NewGoGZipCompressor(output io.Writer, level CompressionLevel, bufferSize uint32) (io.WriteCloser, error) {
	twh := &transformerWriterHandler{
		writtenBytes:     0,
		eventHandlers:    nil,
		eventHandlersPtr: nil,
	}

	goComp := &goGZipCompressor{
		goZLibTransformer{
			input:       nil,
			output:      output,
			transformer: nil,
			twh:         twh,
		},
	}

	err := initTransformer(&goComp.goZLibTransformer, TransformModeGZip, level, bufferSize)

	twh.eventHandlers.onWrite = func(compressed []byte) uint32 {
		written, werr := goComp.output.Write(compressed)
		if werr != nil {
			return 0
		}
		return uint32(written)
	}

	if err != nil {
		return nil, err
	}
	return goComp, nil
}

// Write compresses and writes the given data to the output stream. Returns the
// number of uncompressed bytes written, and any error that occurred.
func (comp *goGZipCompressor) Write(data []byte) (int, error) {
	dataLen := len(data)
	uncompressedLen := C.uInt(dataLen)

	var uncompressed unsafe.Pointer = nil
	if dataLen > 0 {
		uncompressed = unsafe.Pointer(&data[0])
	}

	transformCode := C.go_transformer_compress_to_outstream(comp.transformer, uncompressed, uncompressedLen)

	if transformCode < C.Z_OK {
		return 0, fmt.Errorf(wrapErrorFormat, TransformerCompressionError, transformCode)
	}

	return dataLen, nil
}

// Flush flushes the compressor by invoking Write with a zero input. If there is
// any error during writing, it will be returned.
func (comp *goGZipCompressor) Flush() error {
	// flush by invoking write with zero input
	_, ferr := comp.Write(nil)

	return ferr
}

// Close releases the resources used by the compressor. It first flushes the compressor,
// then releases all interenal resources. If there
// is any error during flushing or releasing, it will be returned.
// Not calling Close will result in a resource leak
func (comp *goGZipCompressor) Close() error {
	ferr := comp.Flush()
	C.release_compression_transformer(comp.transformer)
	unregisterStreamEventHandler(comp.twh.eventHandlersPtr)
	C.pool_free(comp.twh.eventHandlersPtr)
	return ferr
}

type goUncompressor struct {
	goZLibTransformer
	hasMoreData bool
}

// NewGoZLibUncompressor creates a new uncompressor that supports zlib or gzip inputs
// The input parameter is the io.Reader providing the compressed data to be uncompressed,
// and the bufferSize parameter is the size of the buffer to use in the internal compression transformer.
// For best performance, set it to a size that's power 2,
// large enough for the expected input.
func NewGoZLibUncompressor(input io.Reader, bufferSize uint32) (io.ReadCloser, error) {
	twh := &transformerWriterHandler{
		writtenBytes:     0,
		eventHandlers:    nil,
		eventHandlersPtr: nil,
	}

	goUncomp := &goUncompressor{
		goZLibTransformer: goZLibTransformer{
			output:      nil,
			input:       input,
			transformer: nil,
			twh:         twh,
		},
		hasMoreData: false,
	}

	// no need for level when uncompressing so we set it to zero
	err := initTransformer(&goUncomp.goZLibTransformer, TransformModeUncompress, 0, bufferSize)

	// we want to write directly into the output buffer
	// so this handler only tracks the amount written, the actual content
	// is written by the C code to output
	twh.eventHandlers.onWrite = func(data []byte) uint32 {
		twh.writtenBytes = len(data)
		return uint32(twh.writtenBytes)
	}

	if err != nil {
		return nil, err
	}
	return goUncomp, nil
}

// Read reads uncompressed data from the input stream and writes it to the output buffer.
// The function returns the number of bytes read into the output buffer and any error encountered.
// If there is no more data to be read, Read returns io.EOF.
func (unc *goUncompressor) Read(output []byte) (int, error) {
	unc.twh.writtenBytes = 0
	// if there's still data from the previous call to be read
	if !unc.hasMoreData {
		readLen, readError := unc.readIntoWorkBuffer()
		if readError != nil { // this could be EOF
			return 0, readError
		}

		if readLen == 0 {
			return unc.twh.writtenBytes, nil
		}

		// assign the workbuffer as next input
		C.go_assign_uncompress_input(unc.transformer, C.uInt(readLen))
	}

	// pass the pointer to the output slice so the C code can write directly to it
	outputSliceHdr := (*reflect.SliceHeader)(unsafe.Pointer(&output))
	transformCode := C.go_uncompress_to_outstream_step(unc.transformer, unsafe.Pointer(outputSliceHdr.Data), C.uInt(outputSliceHdr.Len))

	if transformCode < C.Z_OK {
		return 0, fmt.Errorf(wrapErrorFormat, TransformerUncompressionError, transformCode)
	}

	if transformCode == C.Z_STREAM_END {
		return unc.twh.writtenBytes, nil
	}

	unc.hasMoreData = transformCode == C.GOZLIB_STREAM_OUTPUT_HAS_MORE_DATA

	return unc.twh.writtenBytes, nil
}

// Close closes the uncompressor and releases internal resources
// Not calling Close will result in a resource leak
func (unc *goUncompressor) Close() error {
	C.release_uncompression_transformer(unc.transformer)
	unregisterStreamEventHandler(unc.twh.eventHandlersPtr)
	C.pool_free(unc.twh.eventHandlersPtr)
	return nil
}

// Transform utility functions

// Flush is a helper function to flush a compressor given an interface
func Flush(compressor io.WriteCloser) error {
	return compressor.(*goGZipCompressor).Flush()
}

// ResetCompressor is a helper function that can be used when pooling compressors
// The compressor will use the given output to write data to
func ResetCompressor(output io.Writer, compressor io.WriteCloser) {
	goComp := compressor.(*goGZipCompressor)
	goComp.output = output
	C.reset_compression_transformer(goComp.transformer)
}

// ResetUncompressor is a helper function that can be used when pooling uncompressors
// the uncompressor will use the given input to read data from
func ResetUncompressor(input io.Reader, uncompressor io.ReadCloser) {
	goUncomp := uncompressor.(*goUncompressor)
	goUncomp.input = input
	C.reset_uncompression_transformer(goUncomp.transformer)
}

func (unc *goUncompressor) readIntoWorkBuffer() (uint32, error) {
	var output []byte

	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&output))

	hdr.Data = uintptr(unc.transformer.work_buffer)
	hdr.Len = int(unc.transformer.work_buffer_cap)
	hdr.Cap = int(unc.transformer.work_buffer_cap)

	readLen, readError := unc.input.Read(output)
	if readError == io.EOF && readLen > 0 {
		return uint32(readLen), nil
	}
	return uint32(readLen), readError
}

func initTransformer(goTransformer *goZLibTransformer, mode TransformMode, level CompressionLevel, bufferSize uint32) error {

	var errorCode C.int = 0
	if mode == TransformModeGZip {
		// the result of acquire_gzip_compression_transformer won't be nil even on error
		// and the result needs to be released on close
		goTransformer.transformer = C.acquire_gzip_compression_transformer(C.int(level), C.uInt(bufferSize), &errorCode)
	} else if mode == TransformModeUncompress {
		goTransformer.transformer = C.acquire_uncompression_transformer(C.uInt(bufferSize), &errorCode)
	} else {
		return fmt.Errorf("mode %v not supported", mode)
	}

	if errorCode != C.Z_OK {
		return fmt.Errorf(wrapErrorFormat, TransformerInitializationError, errorCode)
	}

	eventHandlers := &streamEventHandlers{}
	goTransformer.twh.eventHandlers = eventHandlers

	goTransformer.twh.eventHandlersPtr = C.pool_alloc(uintptrSize)
	// use the address of the C allocated pointer itself as ID
	goTransformer.transformer.state.data_handler = goTransformer.twh.eventHandlersPtr
	registerStreamEventHandler(goTransformer.twh.eventHandlersPtr, eventHandlers)
	return nil
}

// Streaming

func goCompressOrUncompressStream(compress bool, level CompressionLevel, inputBufferSize uint32, outputBufferSize uint32, inputReader DataStreamEventHandler, outputWriter DataStreamEventHandler) (uint64, error) {
	zState := C.pool_acquire_zstream_state()
	defer C.pool_release_zstream_state(zState)

	handlers := &streamEventHandlers{}
	handlers.onRead = inputReader
	handlers.onWrite = outputWriter

	handlersPtr := C.pool_alloc(uintptrSize)
	defer C.pool_free(handlersPtr)
	// use the address of the C allocated pointer itself as ID
	zState.data_handler = handlersPtr
	registerStreamEventHandler(handlersPtr, handlers)
	defer unregisterStreamEventHandler(handlersPtr)

	var errorCode C.int = C.Z_OK
	var outLen C.ulong
	if compress {
		outLen = C.go_gzip_compress_stream(zState, C.int(level), C.uInt(inputBufferSize), C.uInt(outputBufferSize), &errorCode)
		if errorCode != C.Z_OK {
			return 0, fmt.Errorf(wrapErrorFormat, StreamCompressError, errorCode)
		}
	} else {
		outLen = C.go_uncompress_stream(zState, C.uInt(inputBufferSize), C.uInt(outputBufferSize), &errorCode)
		if errorCode != C.Z_OK {
			return 0, fmt.Errorf(wrapErrorFormat, StreamUncompressError, errorCode)
		}
	}

	return uint64(outLen), nil
}

// GoGZipCompressStream compresses a stream of data
// The compression level can be CompressionLevelBestCompression or CompressionLevelBestSpeed
// `inputReader` is a function used to read uncompressed data
// `outputWriter` is a function that takes the compressed data
// `inputBufferSize` and `outputBufferSize` are the sizes of the internal work buffers. For best performance, use large enough power of 2 sizes
// The function returns the number of bytes written to the output stream and an error, if any.
func GoGZipCompressStream(level CompressionLevel, inputBufferSize uint32, outputBufferSize uint32, inputReader DataStreamEventHandler, outputWriter DataStreamEventHandler) (uint64, error) {
	return goCompressOrUncompressStream(true, level, inputBufferSize, outputBufferSize, inputReader, outputWriter)
}

// GoUncompressStream uncompresses a stream of data in gzip or standard zlib format
// `inputReader` is a function used to read compressed data
// `outputWriter` is a function that takes the uncompressed data
// `inputBufferSize` and `outputBufferSize` are the sizes of the internal work buffers. For best performance, use large enough power of 2 sizes
// The function returns the number of bytes written to the output stream and an error, if any.
func GoUncompressStream(inputBufferSize uint32, outputBufferSize uint32, inputReader DataStreamEventHandler, outputWriter DataStreamEventHandler) (uint64, error) {
	return goCompressOrUncompressStream(false, 0, inputBufferSize, outputBufferSize, inputReader, outputWriter)
}

// Buffer to buffer operations

// GoGZipCompressBuffer compresses data in gzip format, reading from input and
// writing to a pre allocated output buffer. If the output is too small to contain the compressed data, an error is returned
func GoGZipCompressBuffer(level CompressionLevel, input []byte, output []byte) (uint64, error) {
	inputCap := cap(input)
	outputCap := cap(output)
	if outputCap == 0 {
		return 0, OutputBufferTooSmallError
	}

	var inputPtr unsafe.Pointer = nil
	if inputCap > 0 {
		inputPtr = unsafe.Pointer(&input[0])
	}

	outputHdr := (*reflect.SliceHeader)(unsafe.Pointer(&output))
	outputPtr := unsafe.Pointer(outputHdr.Data)

	var errorCode C.int = C.Z_OK

	compLen := C.gzip_compress_buffer(C.int(level), inputPtr, C.uInt(inputCap), outputPtr, C.uInt(outputCap), &errorCode)

	if errorCode != C.Z_OK {
		return 0, fmt.Errorf(wrapErrorFormat, BufferCompressError, errorCode)
	}

	return uint64(compLen), nil
}

// GoUncompressBuffer uncompresses zipb or an input buffer writing to a pre allocated output
// if the output is too small to contain the compressed data, an error is returned
func GoUncompressBuffer(input []byte, output []byte) (uint64, error) {
	inputCap := cap(input)
	outputCap := cap(output)
	if outputCap == 0 {
		return 0, OutputBufferTooSmallError
	}

	var inputPtr unsafe.Pointer = nil
	if inputCap > 0 {
		inputPtr = unsafe.Pointer(&input[0])
	}

	outputHdr := (*reflect.SliceHeader)(unsafe.Pointer(&output))
	outputPtr := unsafe.Pointer(outputHdr.Data)

	var errorCode C.int = C.Z_OK

	uncompLen := C.uncompress_buffer_any(inputPtr, C.uInt(inputCap), outputPtr, C.uInt(outputCap), &errorCode)

	if errorCode != C.Z_OK {
		return 0, fmt.Errorf(wrapErrorFormat, BufferUncompressError, errorCode)
	}

	return uint64(uncompLen), nil
}

// native slice pool

// NativeSlicePool is a byte slice pool manager where memory allocated for each slice is allocated off-heap
// The pool allows for slices of various types to be allocated and returned but given the way memory is internally tracked
// slices of sizes that are power of 2 provide an optimal memory utilization.
type NativeSlicePool struct {
	pool *C.struct_MultiPool
}

// NewNativeSlicePool creates a new slice pool.
// Manually call NewNativeSlicePool.Free() to release the resouces allocated by the returned NewNativeSlicePool.
func NewNativeSlicePool() *NativeSlicePool {
	return &NativeSlicePool{
		pool: C.multipool_create(),
	}
}

// Acquire acquires a new byte array. For optimal memory utilization use sizes that are power of 2
// The maximum size of a slice is limited to 4Mb and the returned slice cannot have its capacity changed.
// The returned slice is not zeroed out and it has length zero but capacity equals to size
func (nsp *NativeSlicePool) Acquire(size int) []byte {
	data := C.multipool_mem_acquire(nsp.pool, C.uint32_t(size))

	var slice []byte
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&slice))

	hdr.Data = uintptr(data)
	hdr.Len = 0
	hdr.Cap = int(size)

	return slice
}

// Return returns the slice to the pool.
func (nsp *NativeSlicePool) Return(slice []byte) {
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&slice))

	C.pool_mem_return(unsafe.Pointer(hdr.Data))
}

// Free releases the resources allocated by this pool
// It must be invoked once the pool is not in use anymore to avoid resource leaks
func (nsp *NativeSlicePool) Free() {
	C.multipool_free(nsp.pool)
}
