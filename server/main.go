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

var storageDir string

func main() {
	// Initialize storage directory
	storageDir = filepath.Join(".", "storage")
	os.MkdirAll(storageDir, os.ModePerm)

	// Start QUIC server
	tlsConfig := generateTLSConfig()
	addr := "0.0.0.0:4242"

	listener, err := quic.ListenAddr(addr, tlsConfig, nil)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	fmt.Printf("Server listening on %s...\n", addr)

	// Accept client connections
	for {
		session, err := listener.Accept(context.Background())
		if err != nil {
			log.Printf("Error accepting session: %v", err)
			continue
		}
		go handleSession(session)
	}
}

func handleSession(session quic.Connection) {
	fmt.Println("Client connected")
	defer session.CloseWithError(0, "Session closed")

	for {
		stream, err := session.AcceptStream(context.Background())
		if err != nil {
			log.Printf("Error accepting stream: %v", err)
			return
		}
		go handleStream(stream)
	}
}

func handleStream(stream quic.Stream) {
	defer stream.Close()
	reader := bufio.NewReader(stream)
	command, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("Failed to read from stream: %v", err)
		return
	}
	command = strings.TrimSpace(command)
	fmt.Printf("Received command: %s\n", command)

	switch {
	case strings.HasPrefix(command, "upd "):
		fileName := strings.TrimPrefix(command, "upd ")
		handleUpload(stream, fileName)
	case strings.HasPrefix(command, "dwd "):
		fileName := strings.TrimPrefix(command, "dwd ")
		handleDownload(stream, fileName)
	default:
		stream.Write([]byte("Unknown command\n"))
	}
}

func handleUpload(stream quic.Stream, fileName string) {
	filePath := filepath.Join(storageDir, fileName)
	file, err := os.Create(filePath)
	if err != nil {
		stream.Write([]byte(fmt.Sprintf("Error: Could not create file: %v\n", err)))
		return
	}
	defer file.Close()

	fmt.Printf("Receiving file: %s\n", fileName)
	written, err := io.Copy(file, stream)
	if err != nil {
		stream.Write([]byte(fmt.Sprintf("Error during upload: %v\n", err)))
		return
	}
	fmt.Printf("File %s uploaded successfully (%d bytes)\n", fileName, written)
	stream.Write([]byte("Upload successful\n"))
}

func handleDownload(stream quic.Stream, fileName string) {
	filePath := filepath.Join(storageDir, fileName)
	file, err := os.Open(filePath)
	if err != nil {
		stream.Write([]byte(fmt.Sprintf("Error: Could not open file: %v\n", err)))
		return
	}
	defer file.Close()

	fileInfo, _ := file.Stat()
	fmt.Printf("Sending file: %s (%d bytes)\n", fileName, fileInfo.Size())
	written, err := io.Copy(stream, file)
	if err != nil {
		log.Printf("Error during file download: %v", err)
		return
	}
	fmt.Printf("File %s sent successfully (%d bytes)\n", fileName, written)
}

func generateTLSConfig() *tls.Config {
	cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
	if err != nil {
		log.Fatalf("Error loading TLS keys: %v", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}
}