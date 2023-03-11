package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/bignacio/gozlib"
)

const bufferSize = 1024 * 8
const requestBufferSize = 1024 * 1024 * 2 // 2M

var emptyBuffer = bytes.NewBuffer([]byte{})

var bufPool = gozlib.NewNativeSlicePool()

var gozlibCompressorPool = sync.Pool{
	New: func() interface{} {
		compressor, err := gozlib.NewGoGZipCompressor(emptyBuffer, gozlib.CompressionLevelBestCompression, bufferSize)

		if err != nil {
			panic(err)
		}

		return compressor
	},
}

func bufferedCopyAll(body io.Reader, w io.Writer) {
	buffer := bufPool.Acquire(requestBufferSize)
	defer bufPool.Return(buffer)

	for {
		n, err := body.Read(buffer[len(buffer):cap(buffer)])
		buffer = buffer[:len(buffer)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}

			if err != nil {
				panic(err)
			}
			break
		}
	}

	readLen := len(buffer)

	written, err := w.Write(buffer[:readLen])

	if written != readLen {
		panic("written lenght is different from read length")
	}

	if err != nil {
		panic(err)
	}

}

func gozlibCompress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	compressor := gozlibCompressorPool.Get().(io.WriteCloser)
	defer gozlibCompressorPool.Put(compressor)

	gozlib.ResetCompressor(w, compressor)

	bufferedCopyAll(r.Body, compressor)

	ferr := gozlib.Flush(compressor)

	if ferr != nil {
		panic(ferr)
	}
}

func stdgzipCompress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	compressor, _ := gzip.NewWriterLevel(w, gzip.BestCompression)
	defer compressor.Close()

	bufferedCopyAll(r.Body, compressor)

	ferr := compressor.Flush()
	if ferr != nil {
		panic(ferr)
	}
}

type ResourceStats struct {
	GCPauseTotal   uint64 `json:"gcpause_total"`
	HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
	HeapObjects    uint64 `json:"heap_objects"`
	NumGC          uint64 `json:"num_gc"`
}

func startStatReporter(runStats *bool) {
	for *runStats {
		mstats := runtime.MemStats{}
		runtime.ReadMemStats(&mstats)
		stats := ResourceStats{
			GCPauseTotal:   mstats.PauseTotalNs,
			HeapAllocBytes: mstats.HeapAlloc,
			HeapObjects:    mstats.HeapObjects,
			NumGC:          uint64(mstats.NumGC),
		}

		data, jerr := json.Marshal(stats)
		if jerr != nil {
			panic(jerr)
		}
		fmt.Println(string(data))

		time.Sleep(time.Second * 15)
	}
}

func startHttpServer(port int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/gozlib", gozlibCompress)
	mux.HandleFunc("/stdgzip", stdgzipCompress)

	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT)

	runStats := true
	go startStatReporter(&runStats)

	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	fmt.Println("Server started on", server.Addr)
	<-sig
	fmt.Println("Shutting down server")
	runStats = false
	server.Shutdown(context.Background())
	server.Close()

}

func main() {
	port := flag.Int("port", 8081, "port number")
	flag.Parse()

	startHttpServer(*port)
}
