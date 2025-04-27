package main

import (
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
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
	photos, _, err := getPhotosByVisualID(visual.ID, 0, 0)
	if err != nil {
		return fmt.Errorf("failed to get photos for visual %d: %w", visual.ID, err)
	}

	for _, photo := range photos {
		fullPath := filepath.Join(localFSDir, photo.FilePath)
		err := os.Remove(fullPath)
		if err != nil && !os.IsNotExist(err) {
			// Log the error but continue with other files
			log.Printf("Warning: failed to delete photo file %s: %v", fullPath, err)
		}
	}

	safeTitle := sanitizeFilename(visual.Title)
	visualDir := filepath.Join(localFSDir, "visuals", safeTitle)

	if isEmpty, err := isDirEmpty(visualDir); err == nil && isEmpty {
		err = os.Remove(visualDir)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove visual directory %s: %w", visualDir, err)
		}
	} else if err != nil {
		log.Printf("Warning: failed to check if directory %s is empty: %v", visualDir, err)
	}

	return nil
}

func isDirEmpty(dir string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Try to read at least one entry
	if err == io.EOF {
		return true, nil
	}
	return false, err // Either not empty or error
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

func getPaginationParams(r *http.Request) (int, int) {
	page := 1
	perPage := 10

	if p := r.URL.Query().Get("page"); p != "" {
		if val, err := strconv.Atoi(p); err == nil && val > 0 {
			page = val
		}
	}

	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if val, err := strconv.Atoi(pp); err == nil && val > 0 && val <= 100 {
			perPage = val
		}
	}

	return page, perPage
}
