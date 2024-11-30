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

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	_, loggedIn := getLoginStatus(r)

	if r.Method == http.MethodPost {
		if !loggedIn {
			http.Error(w, "Unauthorized: You must be logged in to update this page.", http.StatusForbidden)
			return
		}

		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Unable to parse form data.", http.StatusBadRequest)
			return
		}

		content := r.FormValue("content")
		if len(content) > 1000 {
			http.Error(w, "Content is too long.", http.StatusBadRequest)
			return
		}

		err = os.WriteFile("data/serve/about.txt", []byte(content), 0644)
		if err != nil {
			log.Printf("Failed to write to file: %v", err)
			http.Error(w, "Failed to save content.", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/about", http.StatusSeeOther)
		return
	}

	err := TPL.ExecuteTemplate(w, "about.gohtml", loginData{Login: loggedIn})
	if err != nil {
		http.Error(w, "Failed to render template.", http.StatusInternalServerError)
	}
}

func contactHandler(w http.ResponseWriter, r *http.Request) {
	_, loggedIn := getLoginStatus(r)

	if r.Method == http.MethodPost {
		if !loggedIn {
			http.Error(w, "Unauthorized: You must be logged in to update this page.", http.StatusForbidden)
			return
		}

		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Unable to parse form data.", http.StatusBadRequest)
			return
		}

		content := r.FormValue("content")
		if len(content) > 1000 {
			http.Error(w, "Content is too long.", http.StatusBadRequest)
			return
		}

		err = os.WriteFile("data/serve/contact.txt", []byte(content), 0644)
		if err != nil {
			log.Printf("Failed to write to file: %v", err)
			http.Error(w, "Failed to save content.", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/contact", http.StatusSeeOther)
		return
	}

	err := TPL.ExecuteTemplate(w, "contact.gohtml", loginData{Login: loggedIn})
	if err != nil {
		http.Error(w, "Failed to render template.", http.StatusInternalServerError)
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
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/logout", logoutHandler)
	mux.HandleFunc("/upload", uploadHandler)
	mux.Handle("/blob/", fileHandler)
	mux.HandleFunc("/about", aboutHandler)
	mux.HandleFunc("/contact", contactHandler)
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
			err = store("data/portfolio.pdf", parsedFile)
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
