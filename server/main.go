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
        fileNames := strings.Fields(strings.TrimPrefix(command, "dwd "))
        handleMultipleDownloads(stream, fileNames)
    case command == "ls":
        handleLSCommand(stream, storageDir)
    default:
        stream.Write([]byte("Unknown command\n"))
    }
}

func handleMultipleDownloads(stream quic.Stream, fileNames []string) {
    totalFiles := len(fileNames)
    fmt.Printf("Sending %d files...\n", totalFiles)

    filesSent := 0
    for _, fileName := range fileNames {
        if handleDownload(stream, fileName) {
            filesSent++
        }
    }

    fmt.Printf("Sent %d/%d successfully.\n", filesSent, totalFiles)
}


func handleUpload(stream quic.Stream, fileName string) {
    // Create the file for writing
    file, err := os.Create(filepath.Join("storage", fileName))
    if err != nil {
        log.Printf("Error: Could not create file %s for upload: %v\n", fileName, err)
        return
    }
    defer file.Close()

    // Write the data received from the client
    written, err := io.Copy(file, stream)
    if err != nil {
        log.Printf("Error during file upload: %v\n", err)
        return
    }

    fmt.Printf("Uploaded file %s (%d bytes) successfully\n", fileName, written)
}


func handleDownload(stream quic.Stream, fileName string) bool {
    filePath := filepath.Join(storageDir, fileName)

    // Open the file for reading
    file, err := os.Open(filePath)
    if err != nil {
        log.Printf("Error opening file %s: %v", fileName, err)
        stream.Write([]byte(fmt.Sprintf("Error: Could not open file %s\n", fileName)))
        return false
    }
    defer file.Close()

    fileInfo, err := file.Stat()
    if err != nil {
        log.Printf("Error getting file info for %s: %v", fileName, err)
        return false
    }

    fmt.Printf("Sending file: %s (%d bytes)\n", fileName, fileInfo.Size())
    _, err = io.Copy(stream, file)
    if err != nil {
        log.Printf("Error sending file %s: %v", fileName, err)
        return false
    }

    return true
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

func handleLSCommand(stream quic.Stream, storageDir string) {
    files, err := os.ReadDir(storageDir)
    if err != nil {
        stream.Write([]byte(fmt.Sprintf("Error: %v\n", err)))
        return
    }

    var fileList []string
    for _, file := range files {
        if !file.IsDir() {
            fileList = append(fileList, file.Name())
        }
    }

    if len(fileList) == 0 {
        stream.Write([]byte("No files available.\n"))
    } else {
        stream.Write([]byte(strings.Join(fileList, "\n") + "\n"))
    }
}
