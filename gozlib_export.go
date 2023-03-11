package gozlib

import (
	"reflect"
	"sync"
	"unsafe"
)
import "C"

type DataStreamEventHandler func(data []byte) uint32
type streamEventHandlers struct {
	onRead  DataStreamEventHandler
	onWrite DataStreamEventHandler
}

var dataStreamEventHandlersTracker = sync.Map{}

func registerStreamEventHandler(id unsafe.Pointer, shandler *streamEventHandlers) {
	dataStreamEventHandlersTracker.Store(uintptr(id), shandler)
}

func unregisterStreamEventHandler(id unsafe.Pointer) {
	dataStreamEventHandlersTracker.Delete(uintptr(id))
}

const uintptrSize = C.ulong(unsafe.Sizeof(uintptr(0)))

func findStreamEventHandler(ptr unsafe.Pointer) *streamEventHandlers {
	dsEventHandlerId := uintptr(ptr)
	if dsEventHandlerId == 0 {
		panic("event handler id cannot be nil")
	}

	shandlerValue, exists := dataStreamEventHandlersTracker.Load(dsEventHandlerId)
	if !exists {
		panic("event handler not found")
	}

	return shandlerValue.(*streamEventHandlers)
}

//export GoStreamDataInputHandler
func GoStreamDataInputHandler(ptr unsafe.Pointer, buffer unsafe.Pointer, bufferLength uint32) uint32 {
	shandler := findStreamEventHandler(ptr)

	var bufferSlice []byte
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&bufferSlice))

	hdr.Data = uintptr(buffer)
	hdr.Len = int(bufferLength)
	hdr.Cap = int(bufferLength)

	return shandler.onRead(bufferSlice)
}

//export GoStreamDataOutputHandler
func GoStreamDataOutputHandler(ptr unsafe.Pointer, buffer unsafe.Pointer, bufferLength uint32) uint32 {
	shandler := findStreamEventHandler(ptr)

	var bufferSlice []byte
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&bufferSlice))

	hdr.Data = uintptr(buffer)
	hdr.Len = int(bufferLength)
	hdr.Cap = int(bufferLength)

	return shandler.onWrite(bufferSlice)
}
