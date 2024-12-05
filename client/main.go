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
	/* secure QUIC connection to the server */
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	session, err := quic.DialAddr(context.Background(), "127.0.0.1:4242", tlsConfig, nil)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer session.CloseWithError(0, "Client closed")
	fmt.Println("Connected to the server!")

	/* File operation */
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Enter command (upload <local> <remote> | download <remote> <local>): ")
		commandLine, _ := reader.ReadString('\n')
		commandLine = strings.TrimSpace(commandLine)

		// Parse the command
		commandParts := strings.Fields(commandLine)
		if len(commandParts) != 3 {
			fmt.Println("Invalid command. Usage: upload <source> <destination> | download <source> <destination>")
			continue
		}

		command := commandParts[0]
		source := commandParts[1]
		destination := commandParts[2]

		switch command {
		case "upload":
			uploadFile(session, source, destination)
		case "download":
			downloadFile(session, source, destination)
		default:
			fmt.Println("You can use either the 'upload' or 'download' command.")
		}
	}
}

func uploadFile(session quic.Connection, localPath, remotePath string) {
    /* getting absolute path for local file */
    localPath, err := filepath.Abs(localPath)
    if err != nil {
        log.Printf("error resolving local file: %v\n", err)
        return
    }

    /* open the file */
    file, err := os.Open(localPath)
    if err != nil {
        log.Printf("Error opening file for upload: %v\n", err)
        return
    }
    defer file.Close()

    fileInfo, _ := file.Stat()
    fileSize := fileInfo.Size()

    /* Open a quic stream */
    stream, err := session.OpenStreamSync(context.Background())
    if err != nil {
        log.Fatalf("Failed to open QUIC stream: %v", err)
    }
    defer stream.Close()

    /* sending the upload command and file size */
    command := fmt.Sprintf("UPLOAD|%s|%d\n", remotePath, fileSize)
    _, err = stream.Write([]byte(command))
    if err != nil {
        log.Printf("Error sending upload command: %v\n", err)
        return
    }

    fmt.Printf("Uploading %s to %s (%d bytes)...\n", localPath, remotePath, fileSize)

    // Set the stream deadline once at the beginning (it should be enough to apply a single deadline)
    stream.SetDeadline(time.Now().Add(60 * time.Second)) 

    buffer := make([]byte, 8192) // Buffer size set to 8KB
    var written int64
    startTime := time.Now()

    // Loop through the file and write chunks to the QUIC stream
    for {
        n, err := file.Read(buffer)
        if n > 0 {
            _, writeErr := stream.Write(buffer[:n])
            if writeErr != nil {
                log.Printf("Error sending data: %v\n", writeErr)
                return
            }
            written += int64(n)
            printProgress(written, fileSize) // Print progress after each chunk
        }

        if err != nil {
            if err == io.EOF {
                break // All file data has been sent
            }
            log.Printf("Error during file read: %v\n", err)
            return
        }
    }

    // Print upload completion
    fmt.Println("\nUpload completed successfully!")
    fmt.Printf("Elapsed Time: %.2f seconds\n", time.Since(startTime).Seconds())
}


// Download a file from the server to the client
func downloadFile(session quic.Connection, remotePath, localPath string) {
	// Open a QUIC stream
	stream, err := session.OpenStreamSync(context.Background())
	if err != nil {
		log.Fatalf("Failed to open QUIC stream: %v", err)
	}
	defer stream.Close()

	// Send download command
	stream.Write([]byte(fmt.Sprintf("DOWNLOAD %s\n", remotePath)))

	// Receive file metadata (file size)
	buffer := make([]byte, 4096)
	n, err := stream.Read(buffer)
	if err != nil {
		log.Printf("Error receiving file metadata: %v\n", err)
		return
	}

	var fileSize int64
	if _, err := fmt.Sscanf(string(buffer[:n]), "SIZE:%d", &fileSize); err != nil {
		log.Printf("Invalid metadata from server: %v\n", err)
		return
	}

	// Resolve local path and create file
	localPath, err = filepath.Abs(localPath)
	if err != nil {
		log.Printf("Error resolving local path: %v\n", err)
		return
	}

	file, err := os.Create(localPath)
	if err != nil {
		log.Printf("Error creating local file: %v\n", err)
		return
	}
	defer file.Close()

	fmt.Printf("Downloading %s to %s (%d bytes)...\n", remotePath, localPath, fileSize)

	// Receive and write file data in chunks
	var written int64
	startTime := time.Now()
	for {
		n, err := stream.Read(buffer)
		if n > 0 {
			_, writeErr := file.Write(buffer[:n])
			if writeErr != nil {
				log.Printf("Error writing to local file: %v\n", writeErr)
				return
			}
			written += int64(n)
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
	fmt.Println("\nDownload completed successfully!")
	fmt.Printf("Elapsed Time: %.2f seconds\n", time.Since(startTime).Seconds())
}

// Display progress as a percentage
func printProgress(written, total int64) {
	percentage := float64(written) / float64(total) * 100
	fmt.Printf("\rProgress: [%-50s] %.2f%%", strings.Repeat("=", int(percentage/2)), percentage)
}
