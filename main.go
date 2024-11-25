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
	"time"

	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
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

type loginData struct {
	Login bool
}

type aboutData struct {
	Login   bool
	Content string
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

	err := TPL.ExecuteTemplate(w, "index.gohtml", loginData{Login: loggedIn})
	if err != nil {
		http.Error(w, "error templating page", http.StatusInternalServerError)
	}
}

func pageHandler(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		page = 0
	}

	pageSize := 2 // Number of PDF pages to load at once

	files, err := os.ReadDir("data/pdf")
	if err != nil {
		http.Error(w, "Failed to read directory", http.StatusInternalServerError)
		return
	}

	// Filter and sort PDF files
	var pdfFiles []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".pdf") {
			pdfFiles = append(pdfFiles, file.Name())
		}
	}

	// Sort files numerically instead of lexicographically
	sort.Slice(pdfFiles, func(i, j int) bool {
		// Extract numbers from filenames and compare
		numI := extractNumber(pdfFiles[i])
		numJ := extractNumber(pdfFiles[j])
		return numI < numJ
	})

	// Calculate pagination
	start := page * pageSize
	end := start + pageSize
	if end > len(pdfFiles) {
		end = len(pdfFiles)
	}

	if start >= len(pdfFiles) {
		// No more pages
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Prepare response
	var pages []PageInfo
	for i, file := range pdfFiles[start:end] {
		pages = append(pages, PageInfo{
			PageNum: start + i + 1,
			Path:    fmt.Sprintf("/pdf/%s?t=%d", file, time.Now().Unix()),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pages)
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
	_, loggedIn := getLoginStatus(r)
	if !loggedIn {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

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

	err := TPL.ExecuteTemplate(w, "upload.gohtml", loginData{Login: loggedIn})
	if err != nil {
		http.Error(w, "error templating page", http.StatusInternalServerError)
	}
}

func styleSheetHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/styles/style.css")
}

func robotsHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "data/robots.txt")
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	_, loggedIn := getLoginStatus(r)

	if r.Method == http.MethodPost {
		if !loggedIn {
			http.Error(w, "forbidden to do post request", http.StatusForbidden)
			return
		}

		err := r.ParseForm()
		if err != nil {
			log.Printf("aboutHandler: failed to parse form data: %v", err)
			http.Error(w, "error parsing form data", http.StatusInternalServerError)
			return
		}

		file, err := os.OpenFile("data/about.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			log.Printf("aboutHandler: failed to open about content for writing: %v", err)
			http.Error(w, "error storing file object", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		content := r.FormValue("content")

		_, err = file.WriteString(content)
		if err != nil {
			log.Printf("aboutHandler: failed to write content to file: %v", err)
			http.Error(w, "error writing content", http.StatusInternalServerError)
			return
		}
	}
	content, err := os.ReadFile("data/about.txt")
	if err != nil {
		http.Error(w, "error reading about contents", http.StatusInternalServerError)
		return
	}
	data := aboutData{
		Login:   loggedIn,
		Content: string(content),
	}

	err = TPL.ExecuteTemplate(w, "about.gohtml", data)
	if err != nil {
		http.Error(w, "error templating about page", http.StatusInternalServerError)
		return
	}
}

func contactHandler(w http.ResponseWriter, r *http.Request) {
	err := TPL.ExecuteTemplate(w, "contact.gohtml", nil)
	if err != nil {
		http.Error(w, "error templating contact page", http.StatusInternalServerError)
	}
}

func noCacheFileServer(dir string) http.Handler {
	fileServer := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set cache control headers
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		fileServer.ServeHTTP(w, r)
	})
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

	mux := http.NewServeMux()
	mux.HandleFunc("/", mainPageHandler)
	mux.HandleFunc("/api/pages", pageHandler)
	mux.Handle("/pdf/", http.StripPrefix("/pdf/", noCacheFileServer("data/pdf")))
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/logout", logoutHandler)
	mux.HandleFunc("/upload", uploadHandler)
	mux.HandleFunc("/about", aboutHandler)
	mux.HandleFunc("/contact", contactHandler)
	mux.Handle("/favicon.ico", http.NotFoundHandler())
	mux.HandleFunc("/robots.txt", robotsHandler)
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

// Helper function to extract number from filename
func extractNumber(filename string) int {
	// Assuming filename format like "something_1.pdf"
	parts := strings.Split(filename, "_")
	if len(parts) < 2 {
		return 0
	}
	numStr := strings.TrimSuffix(parts[len(parts)-1], ".pdf")
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0
	}
	return num
}

func splitPDFByPage(inputPath string, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	pageCount, err := api.PageCountFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to get page count: %v", err)
	}
	if pageCount == 0 {
		return fmt.Errorf("PDF has no pages")
	}

	if err := api.SplitFile(inputPath, outputDir, 1, nil); err != nil {
		return fmt.Errorf("failed to split into single pages: %v", err)
	}

	return nil
}

func determinePort() string {
	port, ok := os.LookupEnv("SERVER_PORT")
	if !ok {
		port = ":8080"
	}
	return port
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
	return nil
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
	err := r.ParseMultipartForm(10000000) // max 10 megabytes
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
			err = storePDF(parsedFile)
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

func storePDF(parsedFile multipart.File) error {
	tempDir, err := os.MkdirTemp(localBlobDir, "")
	if err != nil {
		return fmt.Errorf("storePDF: error creating a new temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tempFile := tempDir + "/file.pdf"
	err = store(tempFile, parsedFile)
	if err != nil {
		return fmt.Errorf("storePDF: error storing PDF: %v", err)
	}

	err = os.RemoveAll(localBlobDir + "pdf")
	if err != nil {
		return fmt.Errorf("storePDF: error removing path '%s': %v", localBlobDir+"pdf", err)
	}

	err = splitPDFByPage(tempFile, localBlobDir+"pdf")
	if err != nil {
		return fmt.Errorf("storePDF: error storing PDF: %v", err)
	}

	return nil
}
