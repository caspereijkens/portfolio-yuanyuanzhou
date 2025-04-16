package main

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"os"
	"strconv"
	"strings"
	"time"

	"net/http"

	"golang.org/x/crypto/bcrypt"

	"path/filepath"
	"unicode"

	_ "github.com/mattn/go-sqlite3"
	uuid "github.com/satori/go.uuid"
)

var TPL = template.Must(template.ParseGlob("static/html/*.gohtml"))
var DB *sql.DB
var err error
var sessionStore = make(map[string]int)

const localBlobDir = "data/"
const backGroundColor = "fffeec"

type User struct {
	Email          string
	PasswordDigest []byte
}

type Text struct {
	ID        int
	UserID    int
	Title     string
	Content   string
	Timestamp *time.Time
}

type Work struct {
	ID          int       `db:"id"`
	UserID      int       `db:"user_id"`
	Title       string    `db:"title"`
	Description string    `db:"description"`
	CreatedAt   time.Time `db:"created_at"`
	Photos      []string  // Populated separately
}

type loginData struct {
	Login bool
}

type aboutData struct {
	Login   bool
	Content string
}

type listTextData struct {
	Login bool
	Texts []Text
}

type listWorkData struct {
	Login bool
	Works []Work
}

type viewTextData struct {
	Login bool
	Text  Text
}

var allowedMIMETypes = map[string]bool{
	"application/pdf": true,
	"image/jpeg":      true,
	"image/png":       true,
	"image/heic":      true,
}

type FileUploadConfig struct {
	FieldName      string
	AllowedTypes   map[string]bool
	DestinationDir string
	Filename       string // If empty, will generate unique name
	MaxSize        int64
}

func mainPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	_, loggedIn := getLoginStatus(r)

	if r.Method == http.MethodPost {
		if !loggedIn {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}

		config := FileUploadConfig{
			FieldName: "image",
			AllowedTypes: map[string]bool{
				"image/jpeg": true,
				"image/png":  true,
				"image/heic": true,
			},
			Filename:       "profile.jpg",
			DestinationDir: "data/serve",
			MaxSize:        10 << 20, // 10 MB
		}

		if _, err := HandleFileUpload(r, config); err != nil {
			if !errors.Is(err, http.ErrMissingFile) {
				log.Printf("Error processing image upload: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	err := TPL.ExecuteTemplate(w, "index.gohtml", loginData{Login: loggedIn})
	if err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
	}
}

func portfolioHandler(w http.ResponseWriter, r *http.Request) {
	_, loggedIn := getLoginStatus(r)

	if r.Method == http.MethodPost {
		if !loggedIn {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}

		config := FileUploadConfig{
			FieldName:      "file",
			AllowedTypes:   map[string]bool{"application/pdf": true},
			Filename:       "portfolio.pdf",
			DestinationDir: "data/serve",
			MaxSize:        10_000_000,
		}

		if _, err := HandleFileUpload(r, config); err != nil {
			log.Println(err)
			http.Error(w, "error storing file object", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	err := TPL.ExecuteTemplate(w, "portfolio.gohtml", loginData{Login: loggedIn})
	if err != nil {
		http.Error(w, "error templating page", http.StatusInternalServerError)
	}
}

func listTextHandler(w http.ResponseWriter, req *http.Request) {
	userID, loggedIn := getLoginStatus(req)

	if req.Method == http.MethodPost {
		if !loggedIn {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}

		err := req.ParseForm()
		if err != nil {
			http.Error(w, "Unable to parse form data.", http.StatusBadRequest)
			return
		}

		text := Text{
			UserID:  *userID,
			Title:   req.FormValue("title"),
			Content: req.FormValue("content"),
		}
		err = insertText(text)
		if err != nil {
			http.Error(w, "Failed to save text", http.StatusInternalServerError)
			log.Printf("Error inserting text: %v", err)
			return
		}
	}

	texts, err := getTexts()
	if err != nil {
		http.Error(w, "Failed to retrieve texts", http.StatusInternalServerError)
		log.Printf("Error retrieving texts: %v", err)
		return
	}

	err = TPL.ExecuteTemplate(w, "textlist.gohtml", listTextData{Login: loggedIn, Texts: texts})
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func listWorkHandler(w http.ResponseWriter, req *http.Request) {
	userID, loggedIn := getLoginStatus(req)

	if req.Method == http.MethodPost {
		if !loggedIn {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}

		// 1. Prepare work data
		work := Work{
			UserID:      *userID,
			Title:       req.FormValue("title"),
			Description: req.FormValue("description"),
		}

		// 2. Create directory for this work's files
		safeTitle := sanitizeFilename(work.Title)
		workDir := fmt.Sprintf("./data/serve/work/%s", safeTitle)
		if err := os.MkdirAll(workDir, 0755); err != nil {
			http.Error(w, "Failed to create work directory", http.StatusInternalServerError)
			return
		}

		// 3. Process file uploads
		var photoPaths []string
		files := req.MultipartForm.File["photos"]

		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				log.Printf("Error opening uploaded file: %v", err)
				continue
			}
			defer file.Close()

			ext := filepath.Ext(fileHeader.Filename)
			fileName := fmt.Sprintf("%s%s", uuid.NewV4().String(), ext)
			filePath := filepath.Join(workDir, fileName)

			if err := saveFile(file, filePath); err != nil {
				log.Printf("Error saving file: %v", err)
				continue
			}

			photoPaths = append(photoPaths, filepath.Join("work", safeTitle, fileName))
		}

		// 4. Save work to database with photo paths
		work.Photos = photoPaths
		_, err := insertWork(work)
		if err != nil {
			// Cleanup uploaded files if DB fails
			os.RemoveAll(workDir)
			http.Error(w, "Failed to save work", http.StatusInternalServerError)
			log.Printf("Error inserting work: %v", err)
			return
		}

		http.Redirect(w, req, "/work", http.StatusSeeOther)
		return
	}

	// GET request handling (unchanged)
	works, err := getWorks()
	if err != nil {
		http.Error(w, "Failed to retrieve works", http.StatusInternalServerError)
		log.Printf("Error retrieving works: %v", err)
		return
	}

	err = TPL.ExecuteTemplate(w, "worklist.gohtml", listWorkData{Login: loggedIn, Works: works})
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func viewWorkHandler(w http.ResponseWriter, req *http.Request) {
	// Extract ID from URL
	idStr := strings.TrimPrefix(req.URL.Path, "/work/")

	if idStr == "" {
		listWorkHandler(w, req) // Reuse your existing list handler
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid work ID - must be an integer", http.StatusBadRequest)
		return
	}

	// Fetch work with photos
	works, err := getWorks(id)
	if err != nil {
		http.Error(w, "Failed to retrieve work", http.StatusInternalServerError)
		log.Printf("Error retrieving work: %v", err)
		return
	}
	if len(works) == 0 {
		http.NotFound(w, req)
		return
	}
	work := works[0]

	// Check login status
	_, loggedIn := getLoginStatus(req)

	// Handle POST (update)
	if req.Method == http.MethodPost {
		if !loggedIn {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}

		if req.FormValue("_method") == "DELETE" {
			if err := deleteWork(work.ID); err != nil {
				http.Error(w, "Failed to delete work", http.StatusInternalServerError)
				log.Printf("Error deleting work: %v", err)
				return
			}
			// Cleanup files
			go cleanupWorkFiles(work)
			http.Redirect(w, req, "/work", http.StatusSeeOther)
			return
		}

		// Parse form (including multipart for potential new photos)
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "Unable to parse form data", http.StatusBadRequest)
			return
		}

		// Update basic fields
		updatedWork := Work{
			ID:          work.ID,
			UserID:      work.UserID, // Preserve original owner
			Title:       req.FormValue("title"),
			Description: req.FormValue("description"),
		}

		// Process new file uploads if any
		if fileHeaders := req.MultipartForm.File["photos"]; len(fileHeaders) > 0 {
			newPaths, err := saveWorkPhotos(updatedWork.Title, fileHeaders)
			if err != nil {
				http.Error(w, "Failed to save photos", http.StatusInternalServerError)
				log.Printf("Error saving photos: %v", err)
				return
			}
			updatedWork.Photos = append(updatedWork.Photos, newPaths...)
		}

		// Update in database
		if err := updateWork(updatedWork); err != nil {
			http.Error(w, "Failed to update work", http.StatusInternalServerError)
			log.Printf("Error updating work: %v", err)
			return
		}

		// Redirect to avoid resubmission
		http.Redirect(w, req, req.URL.Path, http.StatusSeeOther)
		return
	}

	// Render template
	err = TPL.ExecuteTemplate(w, "work.gohtml", struct {
		Login bool
		Work  Work
	}{
		Login: loggedIn,
		Work:  work,
	})
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

// Helper to save uploaded photos for a work
func saveWorkPhotos(workTitle string, fileHeaders []*multipart.FileHeader) ([]string, error) {
	safeTitle := sanitizeFilename(workTitle)
	workDir := fmt.Sprintf("./data/serve/work/%s", safeTitle)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, err
	}

	var paths []string
	for _, fileHeader := range fileHeaders {
		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("error opening uploaded file: %v", err)
		}
		defer file.Close()

		// Generate unique filename
		ext := filepath.Ext(fileHeader.Filename)
		fileName := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), uuid.NewV4().String(), ext)
		filePath := filepath.Join(workDir, fileName)

		// Save file using our common function
		if err := saveFile(file, filePath); err != nil {
			return nil, fmt.Errorf("error saving file: %v", err)
		}

		paths = append(paths, filepath.Join("work", safeTitle, fileName))
	}
	return paths, nil
}

func viewTextHandler(w http.ResponseWriter, req *http.Request) {
	idStr := strings.TrimPrefix(req.URL.Path, "/text/")

	if idStr == "" {
		listTextHandler(w, req)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid post ID - must be an integer", http.StatusBadRequest)
		return
	}

	texts, err := getTexts(id)
	if err != nil {
		http.Error(w, "Failed to retrieve texts", http.StatusInternalServerError)
		log.Printf("Error retrieving texts: %v", err)
		return
	}

	text := texts[0]
	_, loggedIn := getLoginStatus(req)

	if req.Method == http.MethodPost {
		if !loggedIn {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}

		err := req.ParseForm()
		if err != nil {
			http.Error(w, "Unable to parse form data.", http.StatusBadRequest)
			return
		}

		text.Title = req.FormValue("title")
		text.Content = req.FormValue("content")

		err = updateText(text)
		if err != nil {
			http.Error(w, "Failed to save text", http.StatusInternalServerError)
			log.Printf("Error inserting text: %v", err)
			return
		}
	}

	err = TPL.ExecuteTemplate(w, "text.gohtml", viewTextData{Login: loggedIn, Text: text})
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func loginHandler(w http.ResponseWriter, req *http.Request) {
	_, loggedIn := getLoginStatus(req)
	if loggedIn {
		http.Redirect(w, req, "/", http.StatusSeeOther)
		return
	}
	if req.Method == http.MethodPost {
		email := req.FormValue("email")
		err := addSession(w, email, []byte(req.FormValue("password")))
		if err != nil {
			http.Error(w, "Login failed. Please try again.", http.StatusForbidden)
			return
		}
		http.Redirect(w, req, "/", http.StatusSeeOther)
	}
	err := TPL.ExecuteTemplate(w, "login.gohtml", nil)
	if err != nil {
		http.Error(w, "error templating page", http.StatusInternalServerError)
	}
}

func styleSheetHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/styles/style.css")
}

func contentHandler(templateName, contentFile string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		_, loggedIn := getLoginStatus(req)

		if req.Method == http.MethodPost {
			if !loggedIn {
				http.Error(w, "Unauthorized", http.StatusForbidden)
				return
			}

			err := req.ParseForm()
			if err != nil {
				http.Error(w, "Unable to parse form data.", http.StatusBadRequest)
				return
			}

			content := req.FormValue("content")
			if len(content) > 1000 {
				http.Error(w, "Content is too long.", http.StatusBadRequest)
				return
			}

			err = os.WriteFile(contentFile, []byte(content), 0644)
			if err != nil {
				log.Printf("Failed to write to file: %v", err)
				http.Error(w, "Failed to save content.", http.StatusInternalServerError)
				return
			}
			http.Redirect(w, req, "/about", http.StatusSeeOther)
			return
		}
		err := TPL.ExecuteTemplate(w, templateName, loginData{Login: loggedIn})
		if err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
		}
	}
}

func main() {
	port := determinePort()

	DB, err = sql.Open("sqlite3", "./data/sqlite.DB")
	if err != nil {
		log.Fatal(err)
	}
	defer DB.Close()

	err = configDatabase()
	if err != nil {
		log.Fatalf("Failed to start database: %v", err)
	}

	fileHandler := http.StripPrefix("/blob/", http.FileServer(http.Dir("data/serve")))

	mux := http.NewServeMux()
	mux.HandleFunc("/", mainPageHandler)
	mux.HandleFunc("/text", listTextHandler)
	mux.HandleFunc("/text/", viewTextHandler)
	mux.HandleFunc("/work", listWorkHandler)
	mux.HandleFunc("/work/", viewWorkHandler)
	mux.HandleFunc("/portfolio", portfolioHandler)
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/logout", logoutHandler)
	mux.Handle("/blob/", fileHandler)
	mux.HandleFunc("/about", contentHandler("about.gohtml", "data/serve/about.txt"))
	mux.HandleFunc("/contact", contentHandler("contact.gohtml", "data/serve/contact.txt"))
	mux.Handle("/favicon.ico", http.NotFoundHandler())
	mux.Handle("/robots.txt", AddPrefixHandler("/blob", fileHandler))
	mux.HandleFunc("/style.css", styleSheetHandler)

	srv := &http.Server{
		Addr:         port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	log.Printf("Server starting on %s...", port)
	log.Fatal(srv.ListenAndServe())
}

func AddPrefixHandler(prefix string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = prefix + r.URL.Path
		h.ServeHTTP(w, r)
	})
}

func determinePort() string {
	port, ok := os.LookupEnv("SERVER_PORT")
	if !ok {
		port = "8080"
	}
	return ":" + port
}

func configDatabase() error {
	// Existing users table creation
	createUserTable := `
    CREATE TABLE IF NOT EXISTS users (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        email TEXT NOT NULL UNIQUE,
        password_digest BLOB NOT NULL
    );
    `
	_, err := DB.Exec(createUserTable)
	if err != nil {
		log.Printf("configDatabase: %q: %s\n", err, createUserTable)
		return err
	}

	// Existing texts table creation
	createTextTable := `
    CREATE TABLE IF NOT EXISTS texts (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        user_id INTEGER NOT NULL,
        title TEXT NOT NULL,
        content TEXT NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (user_id) REFERENCES users(id)
    );

    CREATE INDEX IF NOT EXISTS idx_texts_user_id ON texts(user_id);
    CREATE INDEX IF NOT EXISTS idx_texts_created_at ON texts(created_at);
    `
	_, err = DB.Exec(createTextTable)
	if err != nil {
		log.Printf("configDatabase: %q: %s\n", err, createTextTable)
		return err
	}

	// New works table
	createWorkTable := `
    CREATE TABLE IF NOT EXISTS works (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        user_id INTEGER NOT NULL,
        title TEXT NOT NULL,
        description TEXT NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (user_id) REFERENCES users(id)
    );

    CREATE INDEX IF NOT EXISTS idx_works_user_id ON works(user_id);
    CREATE INDEX IF NOT EXISTS idx_works_created_at ON works(created_at);
    `
	_, err = DB.Exec(createWorkTable)
	if err != nil {
		log.Printf("configDatabase: %q: %s\n", err, createWorkTable)
		return err
	}

	// New work_photos table (for storing photo paths)
	createWorkPhotosTable := `
    CREATE TABLE IF NOT EXISTS work_photos (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        work_id INTEGER NOT NULL,
        file_path TEXT NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE CASCADE
    );

    CREATE INDEX IF NOT EXISTS idx_work_photos_work_id ON work_photos(work_id);
    `
	_, err = DB.Exec(createWorkPhotosTable)
	if err != nil {
		log.Printf("configDatabase: %q: %s\n", err, createWorkPhotosTable)
		return err
	}

	return nil
}

func getTexts(id ...int) ([]Text, error) {
	var query string
	var args []any

	query = "SELECT id, title, content, created_at FROM texts"

	if len(id) > 0 {
		query += " WHERE id = $1"
		args = append(args, id[0])
	}

	query += " ORDER BY created_at DESC;"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var texts []Text
	var timestamp time.Time

	for rows.Next() {
		var t Text
		if err := rows.Scan(&t.ID, &t.Title, &t.Content, &timestamp); err != nil {
			return nil, err
		}
		t.Timestamp = &timestamp
		texts = append(texts, t)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return texts, nil
}

func getWorks(id ...int) ([]Work, error) {
	var query string
	var args []any

	// Base query for works
	query = `
				SELECT w.id, w.user_id, w.title, w.description, w.created_at, wp.file_path
        FROM works w
        LEFT JOIN work_photos wp ON w.id = wp.work_id
    `

	if len(id) > 0 {
		query += " WHERE w.id = $1"
		args = append(args, id[0])
	}

	query += " ORDER BY w.created_at DESC;"

	// Execute works query
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("getWorks query failed: %v", err)
	}
	defer rows.Close()

	var works []Work
	currentID := -1

	for rows.Next() {
		var w Work
		var photo sql.NullString
		err := rows.Scan(&w.ID, &w.UserID, &w.Title, &w.Description, &w.CreatedAt, &photo)
		if err != nil {
			return nil, fmt.Errorf("getWorks scan failed: %v", err)
		}

		if currentID != w.ID {
			works = append(works, w)
			currentID = w.ID
		}
		if photo.Valid {
			works[len(works)-1].Photos = append(works[len(works)-1].Photos, photo.String)
		}
	}
	return works, nil
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

func getCredentials(email string) (*int, []byte, error) {
	var userId int
	var passwordDigest []byte
	err := DB.QueryRow("SELECT id, password_digest FROM users WHERE email=?;", email).Scan(&userId, &passwordDigest)
	if err != nil {
		log.Printf("getCredentials: %v\n", err)
		return nil, nil, err
	}
	if passwordDigest == nil {
		log.Println("getCredentials: password digest not found")
		return nil, nil, errors.New("password digest not found")
	}
	return &userId, passwordDigest, nil
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

func insertUser(user User) error {
	sqlStmt := `
		INSERT INTO users (email, password_digest) VALUES (?, ?);
	`
	_, err := DB.Exec(sqlStmt, user.Email, user.PasswordDigest)
	if err != nil {
		return fmt.Errorf("insertUser: %v", err)
	}
	return nil
}

func insertWork(work Work) (int64, error) {
	// Begin transaction
	tx, err := DB.Begin()
	if err != nil {
		return 0, fmt.Errorf("insertWork (begin tx): %v", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO works (user_id, title, description) VALUES (?, ?, ?)`,
		work.UserID, work.Title, work.Description,
	)
	if err != nil {
		return 0, fmt.Errorf("insertWork (insert work): %v", err)
	}

	workID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insertWork (get ID): %v", err)
	}

	if len(work.Photos) > 0 {
		stmt, err := tx.Prepare(`
            INSERT INTO work_photos (work_id, file_path)
            VALUES (?, ?)
        `)
		if err != nil {
			return 0, fmt.Errorf("insertWork (prepare photo stmt): %v", err)
		}
		defer stmt.Close()

		for _, path := range work.Photos {
			if _, err = stmt.Exec(workID, path); err != nil {
				return 0, fmt.Errorf("insertWork (insert photo %s): %v", path, err)
			}
		}
	}

	// Commit
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("insertWork (commit): %v", err)
	}

	return workID, nil
}

func insertText(text Text) error {
	sqlStmt := `
		INSERT INTO texts (user_id, title, content) VALUES (?, ?, ?);
	`
	_, err := DB.Exec(sqlStmt, text.UserID, text.Title, text.Content)
	if err != nil {
		return fmt.Errorf("insertText: %v", err)
	}
	return nil
}

func updateWork(work Work) error {
	// Update works table
	_, err := DB.Exec(`
        UPDATE works
        SET title = ?, description = ?
        WHERE id = ?`,
		work.Title, work.Description, work.ID)
	if err != nil {
		return err
	}

	// Add new photos to work_photos
	if len(work.Photos) > 0 {
		stmt, err := DB.Prepare(`
            INSERT INTO work_photos (work_id, file_path)
            VALUES (?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, path := range work.Photos {
			if _, err := stmt.Exec(work.ID, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateText(text Text) error {
	sqlStmt := `
      UPDATE texts
      SET title = ?, content = ?
      WHERE id = ? AND user_id = ?;
    `
	result, err := DB.Exec(sqlStmt, text.Title, text.Content, text.ID, text.UserID)
	if err != nil {
		return fmt.Errorf("updateText: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("updateText (rows affected): %v", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("no rows updated - either text doesn't exist or user doesn't have permission")
	}

	return nil
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

func HandleFileUpload(r *http.Request, config FileUploadConfig) (string, error) {
	// Parse the multipart form
	if err := r.ParseMultipartForm(config.MaxSize); err != nil {
		return "", fmt.Errorf("error parsing form data: %v", err)
	}

	// Get the file from the form
	file, header, err := r.FormFile(config.FieldName)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return "", nil // No file uploaded is not an error
		}
		return "", fmt.Errorf("error getting file from form: %v", err)
	}
	defer file.Close()

	// Read the first 512 bytes to check MIME type
	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil {
		return "", fmt.Errorf("error reading file for MIME type check: %v", err)
	}

	// Reset file pointer
	if _, err = file.Seek(0, 0); err != nil {
		return "", fmt.Errorf("error resetting file pointer: %v", err)
	}

	// Verify MIME type
	mimeType := http.DetectContentType(buffer)
	allowedTypes := config.AllowedTypes
	if allowedTypes == nil {
		allowedTypes = allowedMIMETypes
	}

	if !allowedTypes[mimeType] {
		return "", fmt.Errorf("uploaded file type %s is not supported", mimeType)
	}

	// Handle special file types
	if mimeType == "image/heic" {
		return "", errors.New("HEIC conversion not implemented")
	}

	// Determine filename
	filename := config.Filename
	if filename == "" {
		ext := filepath.Ext(header.Filename)
		filename = fmt.Sprintf("%s%s", uuid.NewV4().String(), ext)
	}

	// Create destination directory if needed
	if config.DestinationDir != "" {
		if err := os.MkdirAll(config.DestinationDir, 0755); err != nil {
			return "", fmt.Errorf("error creating destination directory: %v", err)
		}
	}

	// Create full path
	filePath := filepath.Join(config.DestinationDir, filename)

	// Save the file
	if err := saveFile(file, filePath); err != nil {
		return "", fmt.Errorf("error saving file: %v", err)
	}

	return filePath, nil
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
	// Replace spaces with underscores
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

func deleteWork(id int) error {
    tx, err := DB.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

		log.Printf("Attempt to delete work with id '%d'", id)
    if _, err := tx.Exec(`DELETE FROM work_photos WHERE work_id = ?`, id); err != nil {
				log.Printf("Failed to delete photos with work id '%d': %v", id, err)
        return err
    }

    if _, err := tx.Exec(`DELETE FROM works WHERE id = ?`, id); err != nil {
				log.Printf("Failed to delete work with id '%d': %v", id, err)
        return err
    }

		if err := tx.Commit(); err != nil {
				log.Printf("Failed to delete work with id '%d': %v", id, err)
		    return err
		}

		log.Printf("Successfully deleted work with id '%d'", id)
    return nil
}

func cleanupWorkFiles(work Work) {
    // Delete the entire work directory
    safeTitle := sanitizeFilename(work.Title)
    workDir := filepath.Join("./data/serve/work", safeTitle)
    if err := os.RemoveAll(workDir); err != nil {
        log.Printf("Error cleaning up work files: %v", err)
    }
}
