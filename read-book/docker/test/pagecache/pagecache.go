package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

func main() {
	// Create a 100MB file
	fd, err := os.Create("./largefile.bin")
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	defer fd.Close()

	data := make([]byte, 1024*1024)
	for i := 0; i < 100; i++ {
		fd.Write(data)
	}
	fmt.Println("Created a 100MB file")

	// Read the file
	fd2, err := os.OpenFile("largefile.bin", os.O_RDONLY, 0644)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}
	defer fd2.Close()
	for {
		_, err = fd2.Read(data)
		if err == io.EOF {
			break
		}
	}

	fmt.Println("Read the 100MB file")

	// Call the free command
	out, err := exec.Command("free", "-h").Output()
	if err != nil {
		fmt.Println("Error running free command:", err)
		return
	}
	fmt.Println("Memory usage:", string(out))

	// Set up a channel to listen for SIGUSR1 signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGUSR1)

	// Wait for the signal
	fmt.Println("Waiting for SIGUSR1 signal...")
	<-sigChan

	var memStats runtime.MemStats

	var all = map[int][]byte{}
	for j := 0; ; j++ {
		// Allocate a large block of memory
		bigSlice := make([]byte, 1024*1024*10) // 1 MB
		for i := range bigSlice {
			bigSlice[i] = 1
		}
		all[j] = bigSlice

		// Print memory statistics
		runtime.ReadMemStats(&memStats)
		fmt.Printf("Alloc = %v MiB\n", memStats.Alloc/1024/1024)
		fmt.Printf("TotalAlloc = %v MiB\n", memStats.TotalAlloc/1024/1024)
		fmt.Printf("Sys = %v MiB\n", memStats.Sys/1024/1024)
		fmt.Printf("NumGC = %v\n", memStats.NumGC)

		// Sleep for a while before allocating more memory
		time.Sleep(1 * time.Second)
	}
}
