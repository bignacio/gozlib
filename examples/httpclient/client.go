package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/bignacio/gozlib"
)

var reqSent uint32 = 0

func makeTestData() []byte {
	size := rand.Intn(1024*1024) + 64
	cardinality := rand.Intn(254) + 1

	data := make([]byte, size, size)
	for i := 0; i < size; i++ {
		data[i] = byte(rand.Intn(cardinality))
	}

	return data
}

func validateResponse(requestData []byte, responseData []byte) bool {
	if len(requestData) != len(responseData) {
		return false
	}

	for i := 0; i < len(requestData); i++ {
		if requestData[i] != responseData[i] {
			return false
		}
	}
	return true
}

func startClient(endpoint string) {
	const bufferSize = 1024 * 8
	client := &http.Client{}
	uncompressor, uncerr := gozlib.NewGoZLibUncompressor(bytes.NewBuffer([]byte{}), bufferSize)

	if uncerr != nil {
		panic(uncerr)
	}

	defer uncompressor.Close()

	for {
		requestData := makeTestData()
		atomic.AddUint32(&reqSent, 1)

		req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestData))
		if err != nil {
			panic(err)
		}

		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}

		gozlib.ResetUncompressor(resp.Body, uncompressor)
		responseData, err := io.ReadAll(uncompressor)
		if err != nil {
			panic(err)
		}

		if !validateResponse(requestData, responseData) {
			panic(fmt.Sprintf("Response validation failed.\nInput: %v\nOutput: %v", requestData, responseData))
		}

		resp.Body.Close()

	}
}

func startAllClients(numWorkers int, address string) {
	for i := 0; i < numWorkers; i++ {
		go startClient(address)
	}

}

func startReporter() {
	lastUpdate := time.Now()

	for {
		if time.Now().Sub(lastUpdate) > time.Minute {
			fmt.Println("Sent", reqSent, "requests in the last minute")
			lastUpdate = time.Now()
			atomic.StoreUint32(&reqSent, 0)
		}
		time.Sleep(time.Second)
	}

}

func main() {
	numWorkers := flag.Int("workers", 8, "number of workers")
	endpoint := flag.String("endpoint", "http://127.0.0.1:8081/gozlib", "server address")

	flag.Parse()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT)

	fmt.Println("Starting", *numWorkers, "workers. Sending requests to", *endpoint)
	startAllClients(*numWorkers, *endpoint)
	go startReporter()
	<-sig
}
