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

//132.235.1.17
func main() {
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	//session, err := quic.DialAddr(context.Background(), "127.0.0.1:4242", tlsConfig, nil)
	session, err := quic.DialAddr(context.Background(), "132.235.1.17:4242", tlsConfig, nil)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer session.CloseWithError(0, "Client closed")

	fmt.Println("================= CLIENT =================")
	fmt.Println("Connected to the server!")
	fmt.Println("\nAvailable Commands:")
	fmt.Println("  - upd <file1> <file2> ... : Upload files")
	fmt.Println("  - dwd <file1> <file2> ... : Download files")
	fmt.Println("  - ls                     : List files on the server")
	fmt.Println("  - exit                   : Terminate connection")
	fmt.Println("==========================================\n")

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Enter command: ")
		command, _ := reader.ReadString('\n')
		command = strings.TrimSpace(command)

		if command == "exit" {
			fmt.Println("Connection terminated.")
			break
		}
		if command == "ls" {
			listFiles(session)
		} else if strings.HasPrefix(command, "upd ") {
			fileNames := strings.Fields(strings.TrimPrefix(command, "upd "))
			uploadFiles(session, fileNames)
		} else if strings.HasPrefix(command, "dwd ") {
			fileNames := strings.Fields(strings.TrimPrefix(command, "dwd "))
			downloadFiles(session, fileNames)
		} else {
			fmt.Println("Unknown command. Use 'upd <file>' to upload, 'dwd <file>' to download, or 'ls' to list files.")
		}
	}
}


// Generate a progress bar for given percentage
func generateProgressBar(percentage int) string {
	completed := percentage / 10
	remaining := 10 - completed
	return fmt.Sprintf("[%s%s] %d%%", strings.Repeat("#", completed), strings.Repeat("-", remaining), percentage)
}


// Handle uploading multiple files
func uploadFiles(session quic.Connection, fileNames []string) {
	for _, fileName := range fileNames {
		fmt.Printf("Uploading file: %s\n", fileName)
		uploadFile(session, fileName)
	}
}

// Upload a single file
func uploadFile(session quic.Connection, fileName string) {
	filePath := filepath.Join("filesToUpload", fileName)

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error: Could not open file %s for upload: %v\n", fileName, err)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		log.Printf("Error: Could not stat file %s: %v\n", fileName, err)
		return
	}
	fileSize := fileInfo.Size()

	stream, err := session.OpenStreamSync(context.Background())
	if err != nil {
		log.Fatalf("Failed to open stream: %v", err)
	}

	header := fmt.Sprintf("upd %s\n", fileName)
	_, err = stream.Write([]byte(header))
	if err != nil {
		log.Printf("Error writing upload header: %v\n", err)
		return
	}

	fmt.Printf("Uploading file: %s (%d bytes)\n", fileName, fileSize)
	buffer := make([]byte, 1024)
	totalWritten := 0

	for {
		bytesRead, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			log.Printf("Error reading file %s: %v\n", fileName, err)
			return
		}
		if bytesRead == 0 {
			break
		}

		bytesWritten, err := stream.Write(buffer[:bytesRead])
		if err != nil {
			log.Printf("Error writing to stream for file %s: %v\n", fileName, err)
			return
		}

		totalWritten += bytesWritten
		percentage := int(float64(totalWritten) / float64(fileSize) * 100)
		fmt.Printf("\r  - %s: %s (%d/%d bytes)", fileName, generateProgressBar(percentage), totalWritten, fileSize)
	}

	fmt.Println("\nUpload completed successfully!")
}

func downloadFiles(session quic.Connection, fileNames []string) {
    totalFiles := len(fileNames)
    fmt.Printf("Downloading %d files...\n", totalFiles)

    stream, err := session.OpenStreamSync(context.Background())
    if err != nil {
        log.Fatalf("Failed to open stream: %v", err)
    }
    defer stream.Close()

    // Send a single `dwd` command with all file names
    command := "dwd " + strings.Join(fileNames, " ")
    stream.Write([]byte(command + "\n"))

    filesDownloaded := 0
    for _, fileName := range fileNames {
        if downloadFile(stream, fileName) { // Pass the same stream
            filesDownloaded++
        }
    }

    fmt.Printf("Downloaded %d/%d successfully.\n", filesDownloaded, totalFiles)
}


func downloadFile(stream quic.Stream, fileName string) bool {
    // Send the download request
    stream.Write([]byte("dwd " + fileName + "\n"))

    // Read the server's response
    buffer := make([]byte, 4096)
    bytesRead, err := stream.Read(buffer)
    if err != nil && err != io.EOF {
        log.Printf("Error reading server response: %v", err)
        return false
    }

    response := strings.TrimSpace(string(buffer[:bytesRead]))
    if strings.HasPrefix(response, "Error:") {
        fmt.Println(response) // Display the server's error message
        return false
    }

    // If the response is OK, proceed with the download
    filePath := filepath.Join("downloadedFiles", fileName)
    downloadDir := "downloadedFiles"
    if _, err := os.Stat(downloadDir); os.IsNotExist(err) {
        if err := os.Mkdir(downloadDir, os.ModePerm); err != nil {
            log.Printf("Error creating 'downloadedFiles' directory: %v", err)
            return false
        }
    }

    file, err := os.Create(filePath)
    if err != nil {
        log.Printf("Error creating file %s: %v", filePath, err)
        return false
    }
    defer file.Close()

    for {
        bytesRead, err := stream.Read(buffer)
        if err != nil && err != io.EOF {
            log.Printf("Error reading from stream for file %s: %v\n", fileName, err)
            return false
        }
        if bytesRead == 0 {
            break
        }

        if _, err := file.Write(buffer[:bytesRead]); err != nil {
            log.Printf("Error writing to file %s: %v", fileName, err)
            return false
        }
    }

    return true
}

func listFiles(session quic.Connection) {
    stream, err := session.OpenStreamSync(context.Background())
    if err != nil {
        log.Fatalf("Failed to open stream: %v", err)
    }
    defer stream.Close()

    // Send the ls command to the server
    stream.Write([]byte("ls\n"))

    // Read the server's response
    buffer := make([]byte, 4096)
    bytesRead, err := stream.Read(buffer)
    if err != nil && err != io.EOF {
        log.Printf("Error reading response: %v\n", err)
        return
    }

    response := strings.TrimSpace(string(buffer[:bytesRead]))
    if response == "" {
        fmt.Println("No files available on the server.")
    } else {
        fmt.Println("Files available on the server:")
        fmt.Println(response)
    }
}