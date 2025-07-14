package main

import (
	"database/sql"
	"fmt"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
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
	filename, err := getLatestCoverFilename()
	if err != nil {
		if err != sql.ErrNoRows {
			http.Error(w, "Failed to fetch cover data", http.StatusInternalServerError)
			return
		}
	}

	visuals, err := getVisuals()
	if err != nil {
		http.Error(w, "Failed to retrieve visuals", http.StatusInternalServerError)
		log.Printf("Error retrieving visuals: %v", err)
		return
	}

	stories, err := getStories()
	if err != nil {
		http.Error(w, "Failed to retrieve stories", http.StatusInternalServerError)
		log.Printf("Error retrieving stories: %v", err)
		return
	}

	const coversDir = "covers"
	originalPath := filepath.Join(coversDir, filename)
	largeThumbPath := thumbnailPath(originalPath, "large")
	mediumThumbPath := thumbnailPath(originalPath, "medium")
	_, loggedIn := getLoginStatus(r)
	data := coverData{
		Login:             loggedIn,
		OriginalCoverPath: originalPath,
		LargeCoverPath:    largeThumbPath,
		MediumCoverPath:   mediumThumbPath,
		Visuals:           visuals,
		Stories:           stories,
	}
	err = TPL.ExecuteTemplate(w, "index.gohtml", data)
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

	filename, err := storeFile(fileHeader, FileUploadConfig{
		AllowedTypes:   allowedImageMIMETypes,
		DestinationDir: fmt.Sprintf("%s/covers", localFSDir),
		Thumbnails:     thumbnailConfigs,
	})
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	_, err = DB.Exec("INSERT INTO covers (file_path) VALUES (?)", filename) // Or `(filename)`
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
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetPortfolio(w http.ResponseWriter, r *http.Request) {
	filePath, err := getLatestPortfolioPath()
	if err != nil {
		http.Error(w, "Failed to fetch portfolio data", http.StatusInternalServerError)
		return
	}
	http.ServeFile(w, r, filepath.Join(localFSDir, "portfolios", filePath))
}

func portfolioUploadHandler(w http.ResponseWriter, r *http.Request) {
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

	http.Redirect(w, r, "/", http.StatusSeeOther)
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
	visuals, err := getVisuals(id)
	if err != nil {
		log.Printf("Error retrieving visual: %v", err)
		http.Error(w, "Failed to retrieve visual work", http.StatusInternalServerError)
		return
	}
	if len(visuals) == 0 {
		log.Printf("Could not find visual with id '%d'", id)
		http.NotFound(w, r)
		return
	}

	_, loggedIn := getLoginStatus(r)

	err = TPL.ExecuteTemplate(w, "visual.gohtml", visualData{Login: loggedIn, Visual: visuals[0]})
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func getVisualUploadConfig(visualDir string) FileUploadConfig {
	return FileUploadConfig{
		AllowedTypes:   allowedImageMIMETypes,
		DestinationDir: visualDir,
		MaxSize:        2_000_000,
		Thumbnails:     thumbnailConfigs,
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

	visual, err := getVisualByID(visualID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Visual not found", http.StatusNotFound)
		} else {
			http.Error(w, "Error fetching visual", http.StatusInternalServerError)
		}
		return
	}

	visual.Title = r.FormValue("title")
	visual.Description = r.FormValue("description")

	visualDir := getVisualBaseDir(visual.ID)

	var newPhotoFilenames []string
	if files := r.MultipartForm.File["photos"]; len(files) > 0 {
		config := getVisualUploadConfig(visualDir)
		for _, fileHeader := range files {
			filename, err := storeFile(fileHeader, config)
			if err != nil {
				log.Printf("Error uploading file: %v", err)
				http.Error(w, "Error storing file", http.StatusInternalServerError)
				return
			}
			newPhotoFilenames = append(newPhotoFilenames, filename)
		}
	}

	err = updateVisual(*visual)
	if err != nil {
		log.Printf("Error updating visual: %v", err)
		http.Error(w, "Failed to update visual work", http.StatusInternalServerError)
		return
	}

	if len(newPhotoFilenames) > 0 {
		err = insertPhotos(visual.ID, newPhotoFilenames)
		if err != nil {
			log.Printf("Error inserting new photos: %v", err)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/visuals/%d", visual.ID), http.StatusSeeOther)
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

func createVisualHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	handlePostVisualPhotos(w, r)
}

func visualsApiHandler(w http.ResponseWriter, r *http.Request) {
	trimmedPath := strings.TrimPrefix(r.URL.Path, "/api/v1/visuals/")
	parts := strings.Split(trimmedPath, "/")

	if len(parts) == 1 && parts[0] == "" {
		if r.Method == http.MethodPost {
			handlePostVisualPhotos(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	visualID, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid visual ID in path", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			handleGetVisualPhotos(w, r, visualID)
		case http.MethodPatch:
			handlePatchVisual(w, r)
		case http.MethodDelete:
			handleDeleteVisual(w, r)
		default:
			http.Error(w, "Method not allowed on this resource", http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) > 1 && parts[1] == "photos" {
		if len(parts) == 2 { // /api/v1/visuals/{id}/photos
			if r.Method == http.MethodGet {
				handleGetVisualPhotos(w, r, visualID)
			} else {
				http.Error(w, "Method not allowed on photos collection", http.StatusMethodNotAllowed)
			}
		} else if len(parts) == 3 { // /api/v1/visuals/{id}/photos/{pid}
			photoID, err := strconv.Atoi(parts[2])
			if err != nil {
				http.Error(w, "Invalid photo ID in path", http.StatusBadRequest)
				return
			}
			if r.Method == http.MethodDelete {
				handleDeleteVisualPhoto(w, r, visualID, photoID)
			} else {
				http.Error(w, "Method not allowed on photo resource", http.StatusMethodNotAllowed)
			}
		}
		return
	}

	http.NotFound(w, r)
}

func handleGetVisualPhotos(w http.ResponseWriter, r *http.Request, visualID int) {
	page, perPage := getPaginationParams(r)
	offset := (page - 1) * perPage

	photos, totalCount, err := getPhotosByVisualID(visualID, offset, perPage)
	if err != nil {
		log.Printf("Error retrieving photos: %v", err)
		http.Error(w, "Failed to retrieve photos", http.StatusInternalServerError)
		return
	}

	photoResponses := make([]photoResponse, len(photos))
	for i, p := range photos {
		photoResponses[i] = photoResponse{
			ID:         p.ID,
			Filename:   p.Filename,
			Thumbnails: generateThumbnailPaths(filepath.Join("visuals", strconv.Itoa(visualID), p.Filename)),
		}
	}

	totalPages := 0
	if totalCount > 0 {
		totalPages = (totalCount + perPage - 1) / perPage
	}
	finalResponse := map[string]any{
		"photos": photoResponses,
		"pagination": map[string]any{
			"total":        totalCount,
			"per_page":     perPage,
			"current_page": page,
			"total_pages":  totalPages,
		},
	}

	respondWithJSON(w, http.StatusOK, finalResponse)
}

func handlePostVisualPhotos(w http.ResponseWriter, r *http.Request) {
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

	vid, err := insertVisual(visual)
	if err != nil {
		http.Error(w, "Failed to save visual", http.StatusInternalServerError)
		log.Printf("Error inserting visual: %v", err)
		return
	}

	visualDir := getVisualBaseDir(vid)
	if err := os.MkdirAll(visualDir, 0755); err != nil {
		log.Printf("Error creating visual directory: %v", err)
		deleteVisual(vid) // Rollback visual creation
		http.Error(w, "Failed to create storage", http.StatusInternalServerError)
		return
	}

	var photoFilenames []string
	files := r.MultipartForm.File["photos"]

	for _, fileHeader := range files {
		config := getVisualUploadConfig(visualDir)

		filename, err := storeFile(fileHeader, config)
		if err != nil {
			log.Printf("Error uploading file: %v", err)
			os.RemoveAll(visualDir)
			deleteVisual(vid)
			http.Error(w, "Error storing file", http.StatusInternalServerError)
			return
		}
		photoFilenames = append(photoFilenames, filename)
	}

	if len(photoFilenames) > 0 {
		err = insertPhotos(vid, photoFilenames)
		if err != nil {
			os.RemoveAll(visualDir)
			deleteVisual(vid)
			http.Error(w, "Failed to save photos", http.StatusInternalServerError)
			log.Printf("Error inserting photos: %v", err)
			return
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/visuals/%d", vid), http.StatusSeeOther)
}
func handleDeleteVisualPhoto(w http.ResponseWriter, r *http.Request, visualID int, photoID int) {
	photo, err := getPhotoByID(photoID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Photo not found", http.StatusNotFound)
		} else {
			http.Error(w, "Error fetching photo", http.StatusInternalServerError)
		}
		return
	}

	if photo.VisualID != visualID {
		log.Printf("Security alert: Attempt to delete photo %d from visual %d, but it belongs to visual %d.", photoID, visualID, photo.VisualID)
		http.Error(w, "Forbidden: Photo does not belong to the specified visual.", http.StatusForbidden)
		return
	}

	if err := deletePhoto(photoID); err != nil {
		log.Printf("Error deleting photo record from DB for ID %d: %v", photoID, err)
		http.Error(w, "Failed to delete photo from database", http.StatusInternalServerError)
		return
	}

	visualDir := getVisualBaseDir(visualID)
	photoPath := filepath.Join(visualDir, photo.Filename)

	err = os.Remove(photoPath)
	if err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: Failed to delete photo file at '%s': %v. The database record was deleted.", photoPath, err)
	}

	log.Printf("Successfully deleted photo with id '%d' from visual '%d'", photoID, visualID)
	w.WriteHeader(http.StatusNoContent)
}

func uploadFormHandler(w http.ResponseWriter, r *http.Request) {
	_, loggedIn := getLoginStatus(r)

	uploadType := strings.TrimPrefix(r.URL.Path, "/upload/")
	data := struct {
		Login                    bool
		UploadType               string
		IncludeCompressionScript bool
		Title                    string
	}{
		Login:                    loggedIn,
		UploadType:               uploadType,
		IncludeCompressionScript: uploadType == "cover" || uploadType == "visual",
		Title:                    "Upload " + cases.Title(language.English).String(uploadType),
	}

	err := TPL.ExecuteTemplate(w, "upload-page.gohtml", data)
	if err != nil {
		http.Error(w, "error templating page", http.StatusInternalServerError)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	_, loggedIn := getLoginStatus(r)
	err := TPL.ExecuteTemplate(w, "upload.gohtml", struct{ Login bool }{Login: loggedIn})
	if err != nil {
		http.Error(w, "error templating page", http.StatusInternalServerError)
	}
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

func thumbnailsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetThumbnail(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetThumbnail(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")

	cleanedPath, err := validateAndCleanPath(filePath)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}

	thumbnails := generateThumbnailPaths(cleanedPath)
	respondWithJSON(w, http.StatusOK, thumbnails)
}
