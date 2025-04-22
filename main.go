package main

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var TPL = template.Must(template.ParseGlob("static/html/*.gohtml"))
var err error
var sessionStore = make(map[string]int)
var allowedImageMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/heic": true,
	"image/webp": true,
}

const localFSDir = "data/serve"

type User struct {
	Email          string
	PasswordDigest []byte
}

type Cover struct {
	FilePath string
}

type Portfolio struct {
	FilePath string
}

type Story struct {
	ID        int
	Title     string
	Content   string
	CreatedAt time.Time
}

type Info struct {
	Content string
}

type Visual struct {
	ID          int       `db:"id"`
	Title       string    `db:"title"`
	Description string    `db:"description"`
	CreatedAt   time.Time `db:"created_at"`
	Photos      []string  // Populated separately
}

type loginData struct {
	Login bool
}

type coverData struct {
	Login bool
	Cover Cover
}

type infoData struct {
	Login bool
	Info  Info
}

type portfolioData struct {
	Login     bool
	Portfolio Portfolio
}

type listStoryData struct {
	Login   bool
	Stories []Story
}

type listVisualData struct {
	Login   bool
	Visuals []Visual
}

type storyData struct {
	Login bool
	Story Story
}

type visualData struct {
	Login  bool
	Visual Visual
}

type FileUploadConfig struct {
	AllowedTypes   map[string]bool
	DestinationDir string
	Filename       string
	MaxSize        int64
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

	fileHandler := http.StripPrefix("/fs/", http.FileServer(http.Dir("data/serve")))

	mux := http.NewServeMux()
	mux.HandleFunc("/", requireAuthUnlessGet(indexHandler))
	mux.HandleFunc("/stories/", methodOverride(requireAuthUnlessGet(storiesHandler)))
	mux.HandleFunc("/stories", requireAuthUnlessGet(listStoriesHandler))
	mux.HandleFunc("/visuals/", methodOverride(requireAuthUnlessGet(visualsHandler)))
	mux.HandleFunc("/visuals", requireAuthUnlessGet(listVisualsHandler))
	mux.HandleFunc("/info", methodOverride(requireAuthUnlessGet(infoHandler)))
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/logout", requireAuthUnlessGet(logoutHandler))
	mux.HandleFunc("/portfolio/upload", requireAuth(portfolioUploadHandler))
	mux.HandleFunc("/portfolio", requireAuthUnlessGet(portfolioHandler))
	mux.Handle("/fs/", fileHandler)
	mux.Handle("/favicon.ico", http.NotFoundHandler())
	mux.Handle("/robots.txt", AddPrefixHandler("/fs", fileHandler))
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
