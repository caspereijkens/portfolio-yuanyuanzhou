package main

import (
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"unicode"

	"golang.org/x/crypto/bcrypt"

	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	uuid "github.com/satori/go.uuid"
)

func determinePort() string {
	port, ok := os.LookupEnv("SERVER_PORT")
	if !ok {
		port = "8080"
	}
	return ":" + port
}
func login(email string, password []byte) (*int, error) {
	userId, registeredHashedPassword, err := getCredentials(email)
	if err != nil {
		return nil, err
	}
	err = bcrypt.CompareHashAndPassword(registeredHashedPassword, password)
	if err != nil {
		return nil, err
	}
	return userId, nil
}
func addSession(w http.ResponseWriter, email string, password []byte) error {
	userId, err := login(email, password)
	if err != nil {
		log.Printf("Login failed: %v", err)
		return err
	}
	sessionID := uuid.NewV4().String()
	sessionStore[sessionID] = *userId
	cookie := &http.Cookie{
		Name:  "session",
		Value: sessionID,
	}
	http.SetCookie(w, cookie)
	return nil
}
func saveFile(src multipart.File, dstPath string) error {
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("error creating destination file: %v", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("error copying file contents: %v", err)
	}

	return nil
}

func sanitizeFilename(input string) string {
	output := strings.ReplaceAll(input, " ", "_")
	// Remove any other problematic characters
	output = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-' {
			return r
		}
		return -1
	}, output)
	return output
}

func storeFile(fileHeader *multipart.FileHeader, config FileUploadConfig) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		log.Printf("Error opening uploaded file: %v", err)
		return "", err
	}
	defer file.Close()

	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil {
		return "", fmt.Errorf("error reading file for MIME type check: %v", err)
	}
	mimeType := http.DetectContentType(buffer)
	if !config.AllowedTypes[mimeType] {
		return "", fmt.Errorf("uploaded file type %s is not supported", mimeType)
	}

	if _, err = file.Seek(0, 0); err != nil {
		return "", fmt.Errorf("error resetting file pointer: %v", err)
	}
	filename := config.Filename
	if filename == "" {
		ext := filepath.Ext(fileHeader.Filename)
		filename = fmt.Sprintf("%s%s", uuid.NewV4().String(), ext)
	}
	if config.DestinationDir != "" {
		if err := os.MkdirAll(config.DestinationDir, 0755); err != nil {
			return "", fmt.Errorf("error creating destination directory: %v", err)
		}
	}
	filePath := filepath.Join(config.DestinationDir, filename)
	if err := saveFile(file, filePath); err != nil {
		return "", fmt.Errorf("error saving file: %v", err)
	}

	return strings.TrimPrefix(filePath, localFSDir), nil
}
func cleanupVisualFiles(visual Visual) error {
		for _, path := range visual.Photos {		    
			fullPath := filepath.Join(localFSDir, path)
        
        if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
            return err
        }
        
        dir := filepath.Dir(fullPath)	
				if err := os.Remove(dir); err != nil && !os.IsNotExist(err)  {
					return err
				}
		}

		return nil
}

func deleteSession(req *http.Request) *http.Cookie {
	cookie, err := req.Cookie("session")
	if err != nil {
		return nil
	}
	sessionId := cookie.Value
	delete(sessionStore, sessionId)
	cookie = &http.Cookie{
		Name:   "session",
		Value:  "",
		MaxAge: -1,
	}
	return cookie
}

func getLoginStatus(req *http.Request) (*int, bool) {
	cookie, err := req.Cookie("session")
	if err != nil {
		return nil, false
	}
	sessionId := cookie.Value
	userId, ok := sessionStore[sessionId]
	if !ok {
		return nil, false
	}
	return &userId, true
}

