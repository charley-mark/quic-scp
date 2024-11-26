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

	"github.com/quic-go/quic-go"
)

func main() {
	// Connect to the server
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	session, err := quic.DialAddr(context.Background(), "132.235.1.37:4242", tlsConfig, nil)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer session.CloseWithError(0, "Client closed")

	fmt.Println("Connected to the server!")

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter command (e.g., 'upd <file>' or 'dwd <file>'): ")
	command, _ := reader.ReadString('\n')
	command = strings.TrimSpace(command)

	if strings.HasPrefix(command, "upd ") {
		fileName := strings.TrimPrefix(command, "upd ")
		uploadFile(session, fileName)
	} else if strings.HasPrefix(command, "dwd ") {
		fileName := strings.TrimPrefix(command, "dwd ")
		downloadFile(session, fileName)
	} else {
		fmt.Println("Unknown command. Use 'upd <file>' to upload or 'dwd <file>' to download.")
	}
}

func uploadFile(session quic.Connection, fileName string) {
	filePath := filepath.Join(".", fileName)
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error: Could not open file %s for upload: %v\n", fileName, err)
		return
	}
	defer file.Close()

	stream, err := session.OpenStreamSync(context.Background())
	if err != nil {
		log.Fatalf("Failed to open stream: %v", err)
	}

	stream.Write([]byte("upd " + fileName + "\n"))

	fmt.Printf("Uploading file %s...\n", fileName)
	written, err := io.Copy(stream, file)
	if err != nil {
		log.Printf("Error during upload: %v", err)
		return
	}

	fmt.Printf("Uploaded %d bytes of file %s successfully\n", written, fileName)
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
	written, err := io.Copy(file, stream)
	if err != nil {
		log.Printf("Error during download: %v", err)
		return
	}

	fmt.Printf("Downloaded %d bytes of file %s successfully\n", written, fileName)
}
