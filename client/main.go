package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
)


func main() {
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	session, err := quic.DialAddr(context.Background(), "132.235.1.17:4242", tlsConfig, nil)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer session.CloseWithError(0, "Client closed")

	fmt.Println("Connected to the server!")


    /*
	Parse and excute Command
	*/
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter commandLine (e.g., 'ls', 'upd <file>', 'dwd <file>'): ")
	commandLine, _ := reader.ReadString('\n')
	commandLine = strings.TrimSpace(commandLine)

	if commandLine == "ls" {
		listFiles(session)
	} else if strings.HasPrefix(commandLine, "upd ") {
		filePath := strings.TrimPrefix(commandLine, "upd ")
		uploadFile(session, filePath)
	} else if strings.HasPrefix(commandLine, "dwd ") {
		filePath := strings.TrimPrefix(commandLine, "dwd ")
		downloadFile(session, filePath)
	} else {
		fmt.Println("Unknown commandLine.")
	}
}

func uploadFile(session quic.Connection, fileName string) {
	filePath := filepath.Abs(fileName)
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error: Could not open file %s for upload: %v\n", fileName, err)
		return
	}
	defer file.Close()

	// Get file size
	fileInfo, _ := file.Stat()
	fileSize := fileInfo.Size()

	// Open a QUIC stream
	stream, err := session.OpenStreamSync(context.Background())
	if err != nil {
		log.Fatalf("Failed to open stream: %v", err)
	}

	// Send upload commandLine
	stream.Write([]byte("upd " + fileName + "\n"))

	fmt.Printf("Uploading file %s (%d bytes)...\n", fileName, fileSize)

	// Metrics tracking
	buffer := make([]byte, 1024) // 1 KB chunks
	var written int64            // Bytes written
	packetCount := 0             // Packet counter
	startTime := time.Now()      // Track start time

	for {
		n, err := file.Read(buffer)
		if n > 0 {
			_, writeErr := stream.Write(buffer[:n])
			if writeErr != nil {
				log.Printf("Error sending packet: %v\n", writeErr)
				return
			}
			written += int64(n)
			packetCount++

			// Display progress
			printProgress(written, fileSize)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Error during upload: %v\n", err)
			return
		}
	}

	// Send packet count to the server
	stream.Write([]byte(fmt.Sprintf("PACKETS:%d\n", packetCount)))

	// Close the stream gracefully
	stream.Close()

	// Metrics reporting
	elapsed := time.Since(startTime)
	fmt.Printf("\nUpload completed successfully!\n")
	fmt.Printf("File: %s\n", fileName)
	fmt.Printf("Total Size: %d bytes\n", fileSize)
	fmt.Printf("Elapsed Time: %.2f seconds\n", elapsed.Seconds())
	fmt.Printf("Total Packets Sent: %d\n", packetCount)
	fmt.Printf("Average Throughput: %.2f KB/s\n", float64(written)/elapsed.Seconds()/1024)
}

func printProgress(written, total int64) {
	percentage := float64(written) / float64(total) * 100
	fmt.Printf("\rProgress: [%-50s] %.2f%%", strings.Repeat("=", int(percentage/2)), percentage)
}

func downloadFile(session quic.Connection, fileName string) {
	stream, err := session.OpenStreamSync(context.Background())
	if err != nil {
		log.Fatalf("Failed to open stream: %v", err)
	}

	stream.Write([]byte("dwd " + fileName + "\n"))

	filePath := filepath.Join(".", fileName)
	file, err := os.Create(filePath)
	if err != nil {
		log.Printf("Error: Could not create file %s for download: %v\n", fileName, err)
		return
	}
	defer file.Close()

	fmt.Printf("Downloading file %s...\n", fileName)

	// Read file size from the stream (server should send size first)
	buffer := make([]byte, 1024)
	n, err := stream.Read(buffer)
	if err != nil {
		log.Printf("Error reading file size: %v\n", err)
		return
	}
	fileSize := int64(0)
	fmt.Sscanf(string(buffer[:n]), "SIZE:%d", &fileSize)

	// Download file in chunks with progress
	var written int64
	for {
		n, err := stream.Read(buffer)
		if n > 0 {
			file.Write(buffer[:n])
			written += int64(n)

			// Display progress
			printProgress(written, fileSize)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Error during download: %v\n", err)
			return
		}
	}
	fmt.Printf("\nDownload completed successfully!\n")
}

func listFiles(session quic.Connection) {
	stream, err := session.OpenStreamSync(context.Background())
	if err != nil {
		log.Fatalf("Failed to open stream: %v", err)
	}

	stream.Write([]byte("ls\n"))

	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		log.Printf("Error reading response: %v", err)
	}
	fmt.Printf("Server response:\n%s\n", string(buf[:n]))
}

