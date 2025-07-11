package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
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

	"github.com/disintegration/imaging"
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
	if _, err = file.Read(buffer); err != nil {
		log.Printf("error reading file for MIME type check: %v", err)
		return "", fmt.Errorf("error reading file for MIME type check: %v", err)
	}
	mimeType := http.DetectContentType(buffer)
	if !config.AllowedTypes[mimeType] {
		log.Printf("uploaded file type %s is not supported", mimeType)
		return "", fmt.Errorf("uploaded file type %s is not supported", mimeType)
	}
	if _, err = file.Seek(0, 0); err != nil {
		log.Printf("error resetting file pointer: %v", err)
		return "", fmt.Errorf("error resetting file pointer: %v", err)
	}

	filename := config.Filename
	if filename == "" {
		ext := filepath.Ext(fileHeader.Filename)
		filename = fmt.Sprintf("%s%s", uuid.NewV4().String(), ext)
	}

	if config.DestinationDir != "" {
		if err := os.MkdirAll(config.DestinationDir, 0755); err != nil {
			log.Printf("error creating destination directory: %v", err)
			return "", fmt.Errorf("error creating destination directory: %v", err)
		}
	}

	filePath := filepath.Join(config.DestinationDir, filename)
	if err := saveFile(file, filePath); err != nil {
		log.Printf("error saving file: %v", err)
		return "", fmt.Errorf("error saving file: %v", err)
	}

	if strings.HasPrefix(mimeType, "image/") {
		if err := generateAndSaveThumbnail(filePath, config); err != nil {
			log.Printf("Warning: thumbnail generation failed: %v", err)
		}
	}

	return filename, nil
}

func getVisualBaseDir(vid int) string {
	return filepath.Join(localFSDir, "visuals", strconv.Itoa(vid))
}

func cleanupVisualFiles(visual Visual) error {
	visualDir := getVisualBaseDir(visual.ID)
	err := os.RemoveAll(visualDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove visual directory %s: %w", visualDir, err)
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
	perPage := -1 // Default to -1 (no limit) if not specified

	if p := r.URL.Query().Get("page"); p != "" {
		if val, err := strconv.Atoi(p); err == nil && val > 0 {
			page = val
		}
	}

	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if val, err := strconv.Atoi(pp); err == nil && val > 0 {
			if val <= 100 {
				perPage = val
			} else {
				perPage = 100 // Max per page
			}
		}
	} else {
		// If per_page is not provided, we keep it at -1 to signify no limit.
	}

	return page, perPage
}

var thumbnailConfigs = []ThumbnailConfig{
	{Name: "mini", Width: 40, Quality: 80, Crop: true},
	{Name: "small", Width: 150, Quality: 80},
	{Name: "medium", Width: 600, Quality: 80},
	{Name: "large", Width: 1080, Quality: 80},
}

func generateAndSaveThumbnail(imagePath string, config FileUploadConfig) error {
	img, err := imaging.Open(imagePath, imaging.AutoOrientation(true))
	if err != nil {
		return fmt.Errorf("error opening image for thumbnail: %v", err)
	}

	for _, thumbConfig := range config.Thumbnails {
		if thumbConfig.Crop {
			err = saveCroppedThumbnail(img, thumbConfig.Width, thumbConfig.Name, imagePath, thumbConfig.Quality)
		} else {
			err = saveResizedImage(img, thumbConfig.Width, thumbConfig.Name, imagePath, thumbConfig.Quality)
		}
		if err != nil {
			log.Printf("Warning: failed to generate %s thumbnail for %s: %v", thumbConfig.Name, imagePath, err)
		}
	}

	return nil // Return nil as long as the original file was saved.
}

func saveCroppedThumbnail(img image.Image, size int, sizeName string, originalImagePath string, quality int) error {
	thumb := imaging.Fill(img, size, size, imaging.Center, imaging.Lanczos)
	thumbDir := filepath.Join(filepath.Dir(originalImagePath), "thumbnails", sizeName)
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return fmt.Errorf("error creating %s thumbnail directory: %v", sizeName, err)
	}
	thumbPath := filepath.Join(thumbDir, filepath.Base(originalImagePath))

	// Save as JPEG with a quality setting
	outFile, err := os.Create(thumbPath)
	if err != nil {
		return fmt.Errorf("error creating output file for %s thumbnail: %v", sizeName, err)
	}
	defer outFile.Close()

	// Adjust quality as needed (0-100, higher is better quality but larger size)
	err = jpeg.Encode(outFile, thumb, &jpeg.Options{Quality: quality})
	if err != nil {
		return fmt.Errorf("error saving %s thumbnail as JPEG: %v", sizeName, err)
	}

	return nil
}

func saveResizedImage(img image.Image, width int, sizeName, originalImagePath string, quality int) error {
	resizedImg := imaging.Resize(img, width, 0, imaging.Lanczos)

	thumbDir := filepath.Join(filepath.Dir(originalImagePath), "thumbnails", sizeName)
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return fmt.Errorf("error creating %s thumbnail directory: %v", sizeName, err)
	}

	thumbPath := filepath.Join(thumbDir, filepath.Base(originalImagePath))
	outFile, err := os.Create(thumbPath)
	if err != nil {
		return fmt.Errorf("error creating output file for %s thumbnail: %v", sizeName, err)
	}
	defer outFile.Close()

	// Save as JPEG with a quality setting
	err = jpeg.Encode(outFile, resizedImg, &jpeg.Options{Quality: quality})
	if err != nil {
		return fmt.Errorf("error saving %s thumbnail as JPEG: %v", sizeName, err)
	}
	return nil
}

func thumbnailPath(photoPath, size string) string {
	dir := filepath.Dir(photoPath)
	filename := filepath.Base(photoPath)
	return filepath.Join(dir, "thumbnails", size, filename)
}

func generateThumbnailPaths(originalRelativePath string) thumbnailPaths {
    return thumbnailPaths{
        Mini:   "/fs/" + thumbnailPath(originalRelativePath, "mini"),
        Small:  "/fs/" + thumbnailPath(originalRelativePath, "small"),
        Medium: "/fs/" + thumbnailPath(originalRelativePath, "medium"),
        Large:  "/fs/" + thumbnailPath(originalRelativePath, "large"),
    }
}

func respondWithJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

func validateAndCleanPath(userPath string) (string, error) {
	if userPath == "" {
		return "", fmt.Errorf("missing 'path' query parameter")
	}

	cleanedPath := filepath.Clean(userPath)
	if strings.Contains(cleanedPath, "..") {
		return "", fmt.Errorf("invalid file path (contains '..')")
	}

	fullPath := filepath.Join(localFSDir, cleanedPath)
	if !strings.HasPrefix(fullPath, localFSDir) {
		return "", fmt.Errorf("invalid file path (outside of base directory)")
	}

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return "", fmt.Errorf("source file not found")
	}

	return cleanedPath, nil
}
