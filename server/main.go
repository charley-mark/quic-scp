package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
	"strconv"

	"github.com/quic-go/quic-go"
)

var storageDir string

func main() {
	// Initialize storage directory
	storageDir = filepath.Join(".", "storage")
	os.MkdirAll(storageDir, os.ModePerm)

	// Generate TLS certificates if not present
	generateTLSCert()

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

// handleSession - accepts client connections and processes streams
func handleSession(session quic.Connection) {
    fmt.Println("Client connected")
    defer session.CloseWithError(0, "Session closed")

    for {
        stream, err := session.AcceptStream(context.Background())
        if err != nil {
            // Suppress the error if the client intentionally closed the connection
            if strings.Contains(err.Error(), "Application error 0x0 (remote): Client closed") {
                return // Exit without logging
            }
            // Log unexpected errors
            log.Printf("Unexpected error accepting stream: %v", err)
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
        if strings.Contains(err.Error(), "Application error 0x0 (remote): Client closed") {
            fmt.Println("Stream closed by client.")
            return
        }
        log.Printf("Unexpected error reading from stream: %v", err)
        return
    }
    command = strings.TrimSpace(command)
    fmt.Printf("Received command: %s\n", command)

    // Process the command with error handling
    if err := processCommand(stream, command); err != nil {
        log.Printf("Error processing command: %v\n", err)
        stream.Write([]byte(fmt.Sprintf("Error: %v\n", err)))
    }
}


// Process the command properly in handleStream
func processCommand(stream quic.Stream, command string) error {
    // Check if the command is for uploading
    if strings.HasPrefix(command, "UPLOAD|") {
        // Split the command using "|" as a delimiter
        parts := strings.Split(command, "|")
        if len(parts) != 3 {
            return fmt.Errorf("Invalid upload command: %s", command)
        }

        // Extract the remote path and file size
        remoteFilePath := parts[1] // Destination path on the server
        fileSizeStr := parts[2]    // File size sent by the client as string
        fileSize, err := strconv.ParseInt(fileSizeStr, 10, 64)
        if err != nil || fileSize <= 0 {
            return fmt.Errorf("Invalid file size in command: %s", fileSizeStr)
        }

        fmt.Printf("Preparing to receive file for upload to %s (%d bytes)\n", remoteFilePath, fileSize)

        // Call handleUpload function with stream and the parsed details
        err = handleUpload(stream, remoteFilePath, fileSize)
        if err != nil {
            return fmt.Errorf("Error handling upload: %v", err)
        }
    } else {
        // Handle unknown commands
        stream.Write([]byte("Error: Unknown command\n"))
        return fmt.Errorf("Unknown command: %s", command)
    }
    return nil
}

func handleUpload(stream quic.Stream, remoteFilePath string, fileSize int64) error {
    // Ensure the remote file directory exists
    dir := filepath.Dir(remoteFilePath)
    if err := os.MkdirAll(dir, os.ModePerm); err != nil {
        return fmt.Errorf("Failed to create directories for %s: %v", remoteFilePath, err)
    }

    // Create the file at the specified remote path
    file, err := os.Create(remoteFilePath)
    if err != nil {
        return fmt.Errorf("Failed to create file %s: %v", remoteFilePath, err)
    }
    defer file.Close()

    fmt.Printf("Receiving file data for %s...\n", remoteFilePath)

    buffer := make([]byte, 8192) // 8KB buffer
    var received int64

    // Read the data from the stream
    for {
        n, err := stream.Read(buffer)
        if n > 0 {
            _, writeErr := file.Write(buffer[:n])
            if writeErr != nil {
                return fmt.Errorf("Error writing to file %s: %v", remoteFilePath, writeErr)
            }
            received += int64(n)
            printProgress(received, fileSize) // Print progress
        }

        if err != nil {
            if err == io.EOF {
                break // End of file transmission
            }
            return fmt.Errorf("Error reading from stream: %v", err)
        }
    }

    fmt.Printf("\nUpload complete: %s (%d bytes)\n", remoteFilePath, received)
    return nil
}


func printProgress(received, total int64) {
    percentage := float64(received) / float64(total) * 100
    fmt.Printf("\rProgress: [%-50s] %.2f%%", strings.Repeat("=", int(percentage/2)), percentage)
}


// handleDownload - handles file download from server to client
func handleDownload(stream quic.Stream, filePath string) {
    // Open the file on the server
    remoteFile, err := os.Open(filePath)
    if err != nil {
        stream.Write([]byte(fmt.Sprintf("Error: Could not open remote file at %s: %v\n", filePath, err)))
        return
    }
    defer remoteFile.Close()

    fmt.Printf("Sending file: %s\n", filePath)

    buffer := make([]byte, 8192)
    var packetCount int

    // Read from the file and write it to the stream (send to the client)
    for {
        n, err := remoteFile.Read(buffer)
        if n > 0 {
            _, writeErr := stream.Write(buffer[:n])
            if writeErr != nil {
                log.Printf("Error sending file data: %v", writeErr)
                stream.Write([]byte("Error sending file data\n"))
                return
            }
            packetCount++
        }

        if err != nil {
            if err == io.EOF {
                break
            }
            log.Printf("Error reading file: %v\n", err)
            return
        }
    }

    fmt.Printf("\nFile download completed. Total packets: %d\n", packetCount)
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

// generateTLSCert generates cert.pem and key.pem if they don't exist
func generateTLSCert() {
	// Check if certificates already exist
	if _, err := os.Stat("cert.pem"); err == nil {
		if _, err := os.Stat("key.pem"); err == nil {
			log.Println("TLS certificates already exist. Skipping generation.")
			return
		}
	}

	// Generate private key
	priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate private key: %v", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			Organization: []string{"QUIC SCP Server"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
	}

	// Generate certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		log.Fatalf("Failed to generate certificate: %v", err)
	}

	// Save certificate
	certFile, err := os.Create("cert.pem")
	if err != nil {
		log.Fatalf("Failed to create cert.pem: %v", err)
	}
	defer certFile.Close()
	pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})

	// Save private key
	keyFile, err := os.Create("key.pem")
	if err != nil {
		log.Fatalf("Failed to create key.pem: %v", err)
	}
	defer keyFile.Close()
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		log.Fatalf("Failed to marshal private key: %v", err)
	}
	pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	log.Println("TLS certificates generated successfully.")
}
