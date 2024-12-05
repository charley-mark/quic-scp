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

	switch {
	// case command == "ls":
	// 	handleListFiles(stream)
	case strings.HasPrefix(command, "upload "):
		filePath := strings.TrimPrefix(command, "upload ")
		handleUpload(stream, filePath)
	case strings.HasPrefix(command, "download "):
		filePath := strings.TrimPrefix(command, "download ")
		handleDownload(stream, filePath)
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

	// Metrics tracking
	buffer := make([]byte, 1024) // 1 KB chunks
	var written int64            // Bytes written
	packetCount := 0             // Packet counter

	for {
		n, err := stream.Read(buffer)
		if n > 0 {
			// Check if this is the "PACKETS:<count>" message
			if strings.HasPrefix(string(buffer[:n]), "PACKETS:") {
				// Extract packet count sent by the client
				var clientPacketCount int
				fmt.Sscanf(string(buffer[:n]), "PACKETS:%d", &clientPacketCount)

				// Log sent and received packets
				fmt.Printf("\nFile upload completed.\n")
				fmt.Printf("Total Packets Sent by Client: %d\n", clientPacketCount)
				fmt.Printf("Total Packets Received by Server: %d\n", packetCount)
				if clientPacketCount != packetCount {
					fmt.Printf("WARNING: Packet mismatch! Some packets may have been lost.\n")
				}
				break
			}

			// Write data to file
			_, writeErr := file.Write(buffer[:n])
			if writeErr != nil {
				log.Printf("Error writing to file: %v\n", writeErr)
				stream.Write([]byte("Error writing to file\n"))
				return
			}
			written += int64(n)
			packetCount++ // Increment packet count
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Error receiving packet: %v\n", err)
			return
		}
	}
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
	fileSize := fileInfo.Size()

	// Send file size to the client
	stream.Write([]byte(fmt.Sprintf("SIZE:%d\n", fileSize)))

	fmt.Printf("Sending file: %s (%d bytes)\n", fileName, fileSize)

	// Send file in chunks
	buffer := make([]byte, 1024)
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			stream.Write(buffer[:n])
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Error during file download: %v", err)
			return
		}
	}
	fmt.Printf("File %s sent successfully (%d bytes)\n", fileName, fileSize)
}



func handleListFiles(stream quic.Stream) {
	files, err := os.ReadDir(storageDir)
	if err != nil {
		stream.Write([]byte(fmt.Sprintf("Error reading directory: %v\n", err)))
		return
	}

	var fileList []string
	for _, file := range files {
		fileList = append(fileList, file.Name())
	}
	stream.Write([]byte(strings.Join(fileList, "\n") + "\n"))
	fmt.Println("Listed files in storage directory.")
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