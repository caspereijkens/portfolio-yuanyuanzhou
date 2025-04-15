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

	uuid "github.com/satori/go.uuid"
	"golang.org/x/crypto/bcrypt"

	_ "github.com/mattn/go-sqlite3"
)

var TPL = template.Must(template.ParseGlob("static/html/*.gohtml"))
var DB *sql.DB
var err error
var sessionStore = make(map[string]int)
var localBlobDir = "data/"
var backGroundColor = "fffeec"

type PageInfo struct {
	PageNum int    `json:"pageNum"`
	Path    string `json:"path"`
}

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

type viewTextData struct {
	Login bool
	Text  Text
}

var allowedMIMETypes = map[string]bool{
	"application/pdf": true,
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

		// Parse multipart form (for file uploads)
		err := r.ParseMultipartForm(10 << 20) // 10 MB max
		if err != nil {
			http.Error(w, "Unable to parse form", http.StatusBadRequest)
			return
		}

		// Handle image upload
		file, _, err := r.FormFile("image")
		if err == nil { // File was uploaded
			defer file.Close()

			// Read the first 512 bytes to detect content type
			buffer := make([]byte, 512)
			_, err = file.Read(buffer)
			if err != nil {
				http.Error(w, "Failed to read image", http.StatusInternalServerError)
				return
			}

			// Reset file pointer
			_, err = file.Seek(0, 0)
			if err != nil {
				http.Error(w, "Failed to process image", http.StatusInternalServerError)
				return
			}

			contentType := http.DetectContentType(buffer)
			allowedTypes := map[string]bool{
				"image/jpeg": true,
				"image/png":  true,
				"image/heic": true, // HEIC support would need conversion
			}

			if !allowedTypes[contentType] {
				http.Error(w, "Unsupported image format", http.StatusBadRequest)
				return
			}

			// Convert HEIC to JPEG if needed (would need external library)
			if contentType == "image/heic" {
				// You would need a HEIC decoder library here
				// For example: https://github.com/strukturag/libheif
				http.Error(w, "HEIC conversion not implemented", http.StatusNotImplemented)
				return
			}

			// Create the file
			dst, err := os.Create("data/serve/profile.jpg")
			if err != nil {
				http.Error(w, "Failed to save image", http.StatusInternalServerError)
				return
			}
			defer dst.Close()

			// Copy the uploaded file to the destination
			if _, err := io.Copy(dst, file); err != nil {
				http.Error(w, "Failed to save image", http.StatusInternalServerError)
				return
			}
		} else if !errors.Is(err, http.ErrMissingFile) {
			// Only log if error is something other than "no file uploaded"
			log.Printf("Error processing image upload: %v", err)
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
		err := storeFiles(r)
		if err != nil {
			log.Println(err)
			http.Error(w, "error storing file object ", http.StatusInternalServerError)
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
		http.Redirect(w, req, "/upload", http.StatusSeeOther)
	}
	err := TPL.ExecuteTemplate(w, "login.gohtml", nil)
	if err != nil {
		http.Error(w, "error templating page", http.StatusInternalServerError)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		err := storeFiles(r)
		if err != nil {
			log.Println(err)
			http.Error(w, "error storing file object ", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	_, loggedIn := getLoginStatus(r)
	err := TPL.ExecuteTemplate(w, "upload.gohtml", loginData{Login: loggedIn})
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

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, loggedIn := getLoginStatus(r)
		if !loggedIn {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		next(w, r)
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
	mux.HandleFunc("/portfolio", portfolioHandler)
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/logout", logoutHandler)
	mux.HandleFunc("/upload", authMiddleware(uploadHandler))
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
	createUserTable := `
	CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE,
    password_digest BLOB NOT NULL
	);
	`
	_, err = DB.Exec(createUserTable)
	if err != nil {
		log.Printf("configDatabase: %q: %s\n", err, createUserTable)
		return err
	}

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
	return nil
}

func getTexts(id ...int) ([]Text, error) {
	var query string
	var args []interface{}

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

func storeFiles(r *http.Request) error {
	_, ok := getLoginStatus(r)
	if !ok {
		return fmt.Errorf("request is unauthenticated")
	}
	err := r.ParseMultipartForm(10_000_000) // max 10 megabytes
	if err != nil {
		return fmt.Errorf("error parsing form data: %v", err)
	}

	fileHeaders := r.MultipartForm.File["file"]
	for _, fileHeader := range fileHeaders {
		parsedFile, err := fileHeader.Open()
		if err != nil {
			return fmt.Errorf("storeFiles: error opening file: %v", err)
		}
		defer parsedFile.Close()

		// Read the first 512 bytes to check MIME type
		buffer := make([]byte, 512)
		_, err = parsedFile.Read(buffer)
		if err != nil {
			return fmt.Errorf("storeFiles: error reading file for MIME type check: %v", err)
		}

		mimeType := http.DetectContentType(buffer)
		if !allowedMIMETypes[mimeType] {
			return fmt.Errorf("storeFiles: uploaded file type %s is not supported", mimeType)
		}

		if mimeType == "application/pdf" {
			err = store("data/serve/portfolio.pdf", parsedFile)
		}
	}
	return nil
}

func store(filePath string, parsedFile multipart.File) error {
	dst, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("storeFiles: error creating local file: %v", err)
	}
	defer dst.Close()

	parsedFile.Seek(0, io.SeekStart) // Reset again after hash computation
	_, err = io.Copy(dst, parsedFile)
	if err != nil {
		return fmt.Errorf("storeFiles: error writing to local file: %v", err)
	}
	return nil
}
