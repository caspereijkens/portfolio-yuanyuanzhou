package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		handleGetIndex(w, r)
	case http.MethodPost:
		handlePostIndex(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetIndex(w http.ResponseWriter, r *http.Request) {
	filePath, err := getLatestCoverPath()
	if err != nil {
		http.Error(w, "Failed to fetch cover data", http.StatusInternalServerError)
		return
	}
	cover := Cover{
		FilePath: filePath,
	}
	_, loggedIn := getLoginStatus(r)
	err = TPL.ExecuteTemplate(w, "index.gohtml", coverData{Login: loggedIn, Cover: cover})
	if err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
	}
}

func handlePostIndex(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(2 << 20)
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, fileHeader, err := r.FormFile("cover")
	if err != nil {
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	file.Close()

	contentType := fileHeader.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, fmt.Sprintf("uploaded file type %s is not supported", contentType), http.StatusBadRequest)
		return
	}

	filePath, err := storeFile(fileHeader, FileUploadConfig{
		AllowedTypes:   allowedImageMIMETypes,
		DestinationDir: fmt.Sprintf("./%s/covers", localFSDir),
		MaxSize:        2_000_000,
	})
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	_, err = DB.Exec("INSERT INTO covers (file_path) VALUES (?)", filePath)
	if err != nil {
		http.Error(w, "Failed to update database", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetInfo(w, r)
	case http.MethodPost:
		handlePatchInfo(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetInfo(w http.ResponseWriter, r *http.Request) {
	info, err := getInfo()
	if err != nil {
		http.Error(w, "Failed to fetch cover data", http.StatusInternalServerError)
		return
	}
	_, loggedIn := getLoginStatus(r)
	err = TPL.ExecuteTemplate(w, "info.gohtml", infoData{Login: loggedIn, Info: info})
	if err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
	}
}

func handlePatchInfo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	content := r.FormValue("content")
	if content == "" {
		http.Error(w, "Content cannot be empty", http.StatusBadRequest)
		return
	}

	err := updateInfo(Info{Content: content})
	if err != nil {
		http.Error(w, "Failed to update info", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/info", http.StatusSeeOther)
}

func listStoriesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleListStories(w, r)
	case http.MethodPost:
		handlePostStories(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleListStories(w http.ResponseWriter, r *http.Request) {
	stories, err := getStories()
	if err != nil {
		http.Error(w, "Failed to retrieve stories", http.StatusInternalServerError)
		log.Printf("Error retrieving stories: %v", err)
		return
	}

	_, loggedIn := getLoginStatus(r)
	err = TPL.ExecuteTemplate(w, "stories.gohtml", listStoryData{Login: loggedIn, Stories: stories})
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func handlePostStories(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	content := r.FormValue("content")
	if title == "" || content == "" {
		http.Error(w, "Title and content are required", http.StatusBadRequest)
		return
	}

	story := Story{
		Title:   title,
		Content: content,
	}

	id, err := insertStory(story)
	if err != nil {
		http.Error(w, "Failed to save story", http.StatusInternalServerError)
		log.Printf("Error inserting story: %v", err)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/stories/%d", id), http.StatusSeeOther)
}

func storiesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetStory(w, r)
	case http.MethodPatch:
		handlePatchStory(w, r)
	case http.MethodDelete:
		handleDeleteStory(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetStory(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/stories/")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Redirect(w, r, "/stories", http.StatusSeeOther)
		return
	}

	stories, err := getStories(id)
	if err != nil {
		log.Printf("Error retrieving stories: %v", err)
		http.Error(w, "Failed to retrieve stories", http.StatusInternalServerError)
		return
	}

	story := stories[0]
	_, loggedIn := getLoginStatus(r)
	err = TPL.ExecuteTemplate(w, "story.gohtml", storyData{Login: loggedIn, Story: story})
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func handlePatchStory(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	storyID, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		http.Error(w, "Invalid story ID", http.StatusBadRequest)
		return
	}
	story := Story{
		ID:      storyID,
		Title:   r.FormValue("title"),
		Content: r.FormValue("content"),
	}
	err = updateStory(story)
	if err != nil {
		http.Error(w, "Failed to update story", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/stories/%d", storyID), http.StatusSeeOther)
}

func handleDeleteStory(w http.ResponseWriter, r *http.Request) {
	storyID, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		http.Error(w, "Invalid story ID", http.StatusBadRequest)
		return
	}

	_, err = DB.Exec("DELETE FROM stories WHERE id = ?", storyID)
	if err != nil {
		http.Error(w, "Failed to delete story", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/stories", http.StatusSeeOther)
}

func portfolioHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetPortfolio(w, r)
	case http.MethodPost:
		handlePostPortfolio(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetPortfolio(w http.ResponseWriter, r *http.Request) {
	filePath, err := getLatestPortfolioPath()
	if err != nil {
		http.Error(w, "Failed to fetch cover data", http.StatusInternalServerError)
		return
	}
	http.ServeFile(w, r, filepath.Join(localFSDir, filePath))
}

func handlePostPortfolio(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, fileHeader, err := r.FormFile("portfolio")
	if err != nil {
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	file.Close()

	contentType := fileHeader.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/pdf") {
		http.Error(w, fmt.Sprintf("uploaded file type %s is not supported", contentType), http.StatusBadRequest)
		return
	}

	filePath, err := storeFile(fileHeader, FileUploadConfig{
		AllowedTypes:   map[string]bool{"application/pdf": true},
		DestinationDir: fmt.Sprintf("./%s/portfolios", localFSDir),
		MaxSize:        10_000_000,
	})
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	_, err = DB.Exec("INSERT INTO portfolios (file_path) VALUES (?)", filePath)
	if err != nil {
		http.Error(w, "Failed to update database", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/portfolio", http.StatusSeeOther)
}

func portfolioUploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
  handleUploadPortfolio(w, r)
}

func handleUploadPortfolio(w http.ResponseWriter, r *http.Request) {
	_, loggedIn := getLoginStatus(r)
	err = TPL.ExecuteTemplate(w, "portfolio.gohtml", loginData{Login: loggedIn})
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func visualsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetVisual(w, r)
	case http.MethodPatch:
		handlePatchVisual(w, r)
	case http.MethodDelete:
		handleDeleteVisual(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetVisual(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/visuals/")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Redirect(w, r, "/visuals", http.StatusSeeOther)
		return
	}
	visual, err := getVisuals(id)
	if err != nil {
		log.Printf("Error retrieving visual: %v", err)
		http.Error(w, "Failed to retrieve visual work", http.StatusInternalServerError)
		return
	}

	_, loggedIn := getLoginStatus(r)
	err = TPL.ExecuteTemplate(w, "visual.gohtml", visualData{Login: loggedIn, Visual: visual[0]})
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func handlePatchVisual(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, "Unable to parse form data", http.StatusBadRequest)
		return
	}

	visualID, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		http.Error(w, "Invalid visual ID", http.StatusBadRequest)
		return
	}

	existingVisual, err := getVisuals(visualID)
	if err != nil {
		http.Error(w, "Visual not found", http.StatusNotFound)
		return
	}

	updatedVisual := Visual{
		ID:          visualID,
		Title:       r.FormValue("title"),
		Description: r.FormValue("description"),
		Photos:      existingVisual[0].Photos,
	}
	var newPhotoPaths []string
	safeTitle := sanitizeFilename(updatedVisual.Title)
	visualDir := filepath.Join(localFSDir, "visuals", safeTitle)
	if files := r.MultipartForm.File["photos"]; len(files) > 0 {
		config := FileUploadConfig{
			AllowedTypes:   allowedImageMIMETypes,
			DestinationDir: visualDir,
			MaxSize:        2_000_000,
		}
		for _, fileHeader := range files {
			filePath, err := storeFile(fileHeader, config)
			if err != nil {
				os.RemoveAll(visualDir)
				log.Printf("Error uploading file: %v", err)
				http.Error(w, "Error storing file", http.StatusInternalServerError)
				return
			}
			newPhotoPaths = append(newPhotoPaths, filePath)
		}
		updatedVisual.Photos = newPhotoPaths
	}

	err = updateVisual(updatedVisual)
	if err != nil {
		os.RemoveAll(visualDir)
		log.Printf("Error updating visual: %v", err)
		http.Error(w, "Failed to update visual work", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/visuals/%d", visualID), http.StatusSeeOther)
}

func handleDeleteVisual(w http.ResponseWriter, r *http.Request) {
	visualID, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		http.Error(w, "Invalid visual ID", http.StatusBadRequest)
		return
	}

	visual, err := getVisuals(visualID)
	if err != nil {
		http.Error(w, "Visual not found", http.StatusNotFound)
		return
	}

	err = cleanupVisualFiles(visual[0])
	if err != nil {
		http.Error(w, "Failed to delete visual work", http.StatusInternalServerError)
		log.Printf("Error deleting visual: %v", err)
		return
	}

	err = deleteVisual(visualID)
	if err != nil {
		http.Error(w, "Failed to delete visual work", http.StatusInternalServerError)
		log.Printf("Error deleting visual: %v", err)
		return
	}

	http.Redirect(w, r, "/visuals", http.StatusSeeOther)
}

func listVisualsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleListVisuals(w, r)
	case http.MethodPost:
		handlePostVisual(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleListVisuals(w http.ResponseWriter, r *http.Request) {
	visuals, err := getVisuals()
	if err != nil {
		http.Error(w, "Failed to retrieve visuals", http.StatusInternalServerError)
		log.Printf("Error retrieving visuals: %v", err)
		return
	}

	_, loggedIn := getLoginStatus(r)
	err = TPL.ExecuteTemplate(w, "visuals.gohtml", listVisualData{Login: loggedIn, Visuals: visuals})
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func handlePostVisual(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20) // 10 MB max
	if err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, "Unable to parse form data", http.StatusBadRequest)
		return
	}

	visual := Visual{
		Title:       r.FormValue("title"),
		Description: r.FormValue("description"),
	}

	if visual.Title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	log.Printf("Creating visual - title: %s, description: %s", visual.Title, visual.Description)

	safeTitle := sanitizeFilename(visual.Title)
	visualDir := filepath.Join(localFSDir, "visuals", safeTitle)

	if err := os.MkdirAll(visualDir, 0755); err != nil {
		log.Printf("Error creating visual directory: %v", err)
		http.Error(w, "Failed to create storage", http.StatusInternalServerError)
		return
	}

	var photoPaths []string
	files := r.MultipartForm.File["photos"]

	for _, fileHeader := range files {
		config := FileUploadConfig{
			AllowedTypes:   allowedImageMIMETypes,
			DestinationDir: visualDir,
			MaxSize:        2_000_000,
		}

		filePath, err := storeFile(fileHeader, config)
		if err != nil {
			log.Printf("Error uploading file: %v", err)
			// Clean up any already uploaded files
			os.RemoveAll(visualDir)
			http.Error(w, "Error storing file", http.StatusInternalServerError)
			return
		}
		photoPaths = append(photoPaths, filePath)
	}

	visual.Photos = photoPaths

	id, err := insertVisual(visual)
	if err != nil {
		// Clean up files if DB insert fails
		os.RemoveAll(visualDir)
		http.Error(w, "Failed to save visual", http.StatusInternalServerError)
		log.Printf("Error inserting visual: %v", err)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/visuals/%d", id), http.StatusSeeOther)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	_, loggedIn := getLoginStatus(r)
	if loggedIn {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodPost {
		email := r.FormValue("email")
		err := addSession(w, email, []byte(r.FormValue("password")))
		if err != nil {
			http.Error(w, "Login failed. Please try again.", http.StatusForbidden)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
	err := TPL.ExecuteTemplate(w, "login.gohtml", nil)
	if err != nil {
		http.Error(w, "error templating page", http.StatusInternalServerError)
	}
}

func styleSheetHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/styles/style.css")
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	_, loggedIn := getLoginStatus(r)
	if !loggedIn {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	cookie := deleteSession(r)
	http.SetCookie(w, cookie)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func AddPrefixHandler(prefix string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = prefix + r.URL.Path
		h.ServeHTTP(w, r)
	})
}

func requireAuthUnlessGet(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, loggedIn := getLoginStatus(r)
		if r.Method != http.MethodGet && !loggedIn {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, loggedIn := getLoginStatus(r)
		if !loggedIn {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
func methodOverride(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			switch method := r.PostFormValue("_method"); method {
			case "PATCH":
				r.Method = http.MethodPatch
			case "DELETE":
				r.Method = http.MethodDelete
			case "PUT":
				r.Method = http.MethodPut
			}
		}
		next(w, r)
	}
}
