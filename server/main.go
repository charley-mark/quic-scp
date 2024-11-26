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
