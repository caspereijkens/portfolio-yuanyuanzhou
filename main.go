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
	mux.HandleFunc("/photos/visual/", requireAuthUnlessGet(photosHandler))
	mux.HandleFunc("/photos/", requireAuthUnlessGet(photosHandler))
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
