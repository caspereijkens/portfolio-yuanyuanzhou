package main

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"mime/multipart"
	"os"
	"io"
	"strings"
	"time"

	"net/http"

	"golang.org/x/crypto/bcrypt"

	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	uuid "github.com/satori/go.uuid"
)

var TPL = template.Must(template.ParseGlob("static/html/*.gohtml"))
var err error
var sessionStore = make(map[string]int)
var allowedImageMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/heic": true,
}

const localFSDir = "data/serve"


type User struct {
	Email          string
	PasswordDigest []byte
}

type Cover struct {
	FilePath string
}

//type Story struct {
//	ID        int
//	UserID    int
//	Title     string
//	Content   string
//	Timestamp *time.Time
//}

type Info struct {
	Content   string
}

//type Visual struct {
//	ID          int       `db:"id"`
//	UserID      int       `db:"user_id"`
//	Title       string    `db:"title"`
//	Description string    `db:"description"`
//	CreatedAt   time.Time `db:"created_at"`
//	Photos      []string  // Populated separately
//}

type loginData struct {
	Login bool
}

type coverData struct {
	Login bool
	Cover Cover
}

type infoData struct {
	Login bool
  Info Info
}

//type listStoryData struct {
//	Login   bool
//	Stories []Story
//}
//
//type listVisualData struct {
//	Login   bool
//	Visuals []Visual
//}
//
//type storyData struct {
//	Login bool
//	Story Story
//}
//
//type visualData struct {
//	Login  bool
//	Visual Visual
//}

type FileUploadConfig struct {
	AllowedTypes   map[string]bool
	DestinationDir string
	Filename       string // If empty, will generate unique name
	MaxSize        int64
}

func landingPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		handleGetLanding(w, r)
	case http.MethodPost:
		handlePostLanding(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	} 
}

func handleGetLanding(w http.ResponseWriter, r *http.Request) {
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

func handlePostLanding(w http.ResponseWriter, r *http.Request) {
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
			AllowedTypes: allowedImageMIMETypes,
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

//func infoHandler(w http.ResponseWriter, req *http.Request) {
//	_, loggedIn := getLoginStatus(req)
//
//	err := TPL.ExecuteTemplate(w, "info.gohtml", loginData{Login: loggedIn})
//	if err != nil {
//		http.Error(w, "Failed to render template", http.StatusInternalServerError)
//	}
//}
//
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

////func postHandler(w http.ResponseWriter, req *http.Request) {
////	_, loggedIn := getLoginStatus(req)
////
////	if req.Method == http.MethodPost {
////		if !loggedIn {
////			http.Error(w, "Unauthorized", http.StatusForbidden)
////			return
////		}
////
////		queryParams, err := url.ParseQuery(req.URL.RawQuery)
////		if err != nil {
////			log.Printf("Error parsing query: %v", err)
////			http.Error(w, "Malformatted URL in request", http.StatusBadRequest)
////			return
////		}
////		typeParam := queryParams.Get("type")
////		post(w, req, typeParam)
////	}
////
////	return
////}
//
//func post(w http.ResponseWriter, req *http.Request, typeParam string) {
//	userID, _ := getLoginStatus(req)
//
//	log.Printf("Posting %s", typeParam)
//	switch typeParam {
//	case "visual":
//		err = req.ParseMultipartForm(10 << 20)
//		if err != nil {
//			log.Printf("Error parsing form: %v", err)
//			http.Error(w, "Unable to parse form data.", http.StatusBadRequest)
//			return
//		}
//		visual := Visual{
//			UserID:      *userID,
//			Title:       req.FormValue("title"),
//			Description: req.FormValue("description"),
//		}
//		log.Printf("title: %s, description: %s", visual.Title, visual.Description)
//		safeTitle := sanitizeFilename(visual.Title)
//		visualDir := fmt.Sprintf(".localFSDir/serve/visual/%s", safeTitle)
//		var photoPaths []string
//		log.Println("Just before listing files")
//		files := req.MultipartForm.File["photos"]
//		log.Println("Just before file upload")
//		for _, fileHeader := range files {
//			log.Println("Just before config decl")
//			config := FileUploadConfig{
//				AllowedTypes: map[string]bool{
//					"image/jpeg": true,
//					"image/png":  true,
//					"image/heic": true,
//				},
//				DestinationDir: visualDir,
//				MaxSize:        2_000_000,
//			}
//			filePath, err := storeFile(fileHeader, config)
//			if err != nil {
//				log.Printf("Error uploading file: %v", err)
//				http.Error(w, "error storing file object", http.StatusInternalServerError)
//				return
//			}
//			photoPaths = append(photoPaths, filePath)
//		}
//
//		visual.Photos = photoPaths
//		id, err := insertVisual(visual)
//		if err != nil {
//			os.RemoveAll(visualDir)
//			http.Error(w, "Failed to save visual", http.StatusInternalServerError)
//			log.Printf("Error inserting visual: %v", err)
//			return
//		}
//		http.Redirect(w, req, fmt.Sprintf("/visual/%d", id), http.StatusSeeOther)
//	case "story":
//		story := Story{
//			UserID:  *userID,
//			Title:   req.FormValue("title"),
//			Content: req.FormValue("content"),
//		}
//		id, err := insertStory(story)
//		if err != nil {
//			http.Error(w, "Failed to save story", http.StatusInternalServerError)
//			log.Printf("Error inserting story: %v", err)
//			return
//		}
//		http.Redirect(w, req, fmt.Sprintf("/story/%d", id), http.StatusSeeOther)
//	case "info":
//		info := Info{
//			UserID:  *userID,
//			Content: req.FormValue("content"),
//		}
//		err := updateInfo(info)
//		if err != nil {
//			http.Error(w, "Failed to save info", http.StatusInternalServerError)
//			log.Printf("Error inserting info: %v", err)
//			return
//		}
//		http.Redirect(w, req, "/info", http.StatusSeeOther)
//	case "portfolio":
//		_, header, err := req.FormFile("file")
//		if err != nil {
//			http.Error(w, "Failed to save info", http.StatusInternalServerError)
//			log.Printf("Error getting file from form: %v", err)
//			return
//		}
//		config := FileUploadConfig{
//			AllowedTypes:   map[string]bool{"application/pdf": true},
//			Filename:       "portfolio.pdf",
//			DestinationDir: "data/serve",
//			MaxSize:        10_000_000,
//		}
//		if _, err := storeFile(header, config); err != nil {
//			log.Println(err)
//			http.Error(w, "error storing file object", http.StatusInternalServerError)
//			return
//		}
//		http.Redirect(w, req, "/fs/portfolio.pdf", http.StatusSeeOther)
//	default:
//		log.Printf("Error parsing query: %v", err)
//		http.Error(w, "Malformatted URL in request", http.StatusBadRequest)
//	}
//}
//
//func listStoriesHandler(w http.ResponseWriter, req *http.Request) {
//	_, loggedIn := getLoginStatus(req)
//
//	stories, err := getStories()
//	if err != nil {
//		http.Error(w, "Failed to retrieve stories", http.StatusInternalServerError)
//		log.Printf("Error retrieving stories: %v", err)
//		return
//	}
//
//	err = TPL.ExecuteTemplate(w, "stories.gohtml", listStoryData{Login: loggedIn, Stories: stories})
//	if err != nil {
//		http.Error(w, "Template error", http.StatusInternalServerError)
//	}
//}
//
//func listVisualsHandler(w http.ResponseWriter, req *http.Request) {
//	_, loggedIn := getLoginStatus(req)
//
//	visuals, err := getVisuals()
//	if err != nil {
//		http.Error(w, "Failed to retrieve visuals", http.StatusInternalServerError)
//		log.Printf("Error retrieving visuals: %v", err)
//		return
//	}
//
//	err = TPL.ExecuteTemplate(w, "visuals.gohtml", listVisualData{Login: loggedIn, Visuals: visuals})
//	if err != nil {
//		http.Error(w, "Template error", http.StatusInternalServerError)
//	}
//}
//
//func visualsHandler(w http.ResponseWriter, req *http.Request) {
//	idStr := strings.TrimPrefix(req.URL.Path, "/visual/")
//
//	if idStr == "" {
//		listVisualsHandler(w, req) // Reuse your existing list handler
//		return
//	}
//
//	id, err := strconv.Atoi(idStr)
//	if err != nil {
//		http.Error(w, "Invalid visual ID - must be an integer", http.StatusBadRequest)
//		return
//	}
//
//	visuals, err := getVisuals(id)
//	if err != nil {
//		http.Error(w, "Failed to retrieve visual", http.StatusInternalServerError)
//		log.Printf("Error retrieving visual: %v", err)
//		return
//	}
//	if len(visuals) == 0 {
//		http.NotFound(w, req)
//		return
//	}
//	visual := visuals[0]
//
//	_, loggedIn := getLoginStatus(req)
//
//	err = TPL.ExecuteTemplate(w, "visual.gohtml", visualData{Login: loggedIn, Visual: visual})
//	if err != nil {
//		http.Error(w, "Template error", http.StatusInternalServerError)
//	}
//}
//
//func storiesHandler(w http.ResponseWriter, req *http.Request) {
//	idStr := strings.TrimPrefix(req.URL.Path, "/story/")
//
//	id, err := strconv.Atoi(idStr)
//	if err != nil {
//		http.Redirect(w, req, "/stories", http.StatusSeeOther)
//		return
//	}
//
//	stories, err := getStories(id)
//	if err != nil {
//		log.Printf("Error retrieving stories: %v", err)
//		http.Error(w, "Failed to retrieve stories", http.StatusInternalServerError)
//		return
//	}
//
//	story := stories[0]
//	_, loggedIn := getLoginStatus(req)
//	err = TPL.ExecuteTemplate(w, "story.gohtml", storyData{Login: loggedIn, Story: story})
//	if err != nil {
//		http.Error(w, "Template error", http.StatusInternalServerError)
//	}
//}
//
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
	mux.HandleFunc("/", requireAuthUnlessGet(landingPageHandler))
//	mux.HandleFunc("/stories",  requireAuthUnlessGet(listStoriesHandler))
//	mux.HandleFunc("/stories/",  requireAuthUnlessGet(storiesHandler))
//	mux.HandleFunc("/visuals", requireAuthUnlessGet(listVisualsHandler))
//	mux.HandleFunc("/visuals/", requireAuthUnlessGet(visualsHandler))
	mux.HandleFunc("/info", requireAuthUnlessGet(infoHandler))
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/logout", requireAuthUnlessGet(logoutHandler))
	mux.Handle("/fs/", fileHandler)
	mux.Handle("/favicon.ico", http.NotFoundHandler())
	mux.Handle("/robots.txt", AddPrefixHandler("/fs", fileHandler))
	mux.Handle("/portfolio.pdf", AddPrefixHandler("/fs", fileHandler))
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


//func getStories(id ...int) ([]Story, error) {
//	var query string
//	var args []any
//
//	query = "SELECT id, title, content, created_at FROM stories"
//
//	if len(id) > 0 {
//		query += " WHERE id = $1"
//		args = append(args, id[0])
//	}
//
//	query += " ORDER BY created_at DESC;"
//
//	rows, err := DB.Query(query, args...)
//	if err != nil {
//		return nil, err
//	}
//	defer rows.Close()
//
//	var stories []Story
//	var timestamp time.Time
//
//	for rows.Next() {
//		var t Story
//		if err := rows.Scan(&t.ID, &t.Title, &t.Content, &timestamp); err != nil {
//			return nil, err
//		}
//		t.Timestamp = &timestamp
//		stories = append(stories, t)
//	}
//
//	if err = rows.Err(); err != nil {
//		return nil, err
//	}
//
//	return stories, nil
//}
//
//func getVisuals(id ...int) ([]Visual, error) {
//	var query string
//	var args []any
//
//	// Base query for visuals
//	query = `
//				SELECT w.id, w.user_id, w.title, w.description, w.created_at, wp.file_path
//        FROM visuals w
//        LEFT JOIN visual_photos wp ON w.id = wp.visual_id
//    `
//
//	if len(id) > 0 {
//		query += " WHERE w.id = $1"
//		args = append(args, id[0])
//	}
//
//	query += " ORDER BY w.created_at DESC;"
//
//	// Execute visuals query
//	rows, err := DB.Query(query, args...)
//	if err != nil {
//		return nil, fmt.Errorf("getVisuals query failed: %v", err)
//	}
//	defer rows.Close()
//
//	var visuals []Visual
//	currentID := -1
//
//	for rows.Next() {
//		var w Visual
//		var photo sql.NullString
//		err := rows.Scan(&w.ID, &w.UserID, &w.Title, &w.Description, &w.CreatedAt, &photo)
//		if err != nil {
//			return nil, fmt.Errorf("getVisuals scan failed: %v", err)
//		}
//
//		if currentID != w.ID {
//			visuals = append(visuals, w)
//			currentID = w.ID
//		}
//		if photo.Valid {
//			visuals[len(visuals)-1].Photos = append(visuals[len(visuals)-1].Photos, photo.String)
//		}
//	}
//	return visuals, nil
//}

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

//func insertVisual(visual Visual) (int64, error) {
//	// Begin transaction
//	tx, err := DB.Begin()
//	if err != nil {
//		return 0, fmt.Errorf("insertVisual (begin tx): %v", err)
//	}
//	defer tx.Rollback()
//
//	result, err := tx.Exec(
//		`INSERT INTO visuals (user_id, title, description) VALUES (?, ?, ?)`,
//		visual.UserID, visual.Title, visual.Description,
//	)
//	if err != nil {
//		return 0, fmt.Errorf("insertVisual (insert visual): %v", err)
//	}
//
//	visualID, err := result.LastInsertId()
//	if err != nil {
//		return 0, fmt.Errorf("insertVisual (get ID): %v", err)
//	}
//
//	if len(visual.Photos) > 0 {
//		stmt, err := tx.Prepare(`
//            INSERT INTO visual_photos (visual_id, file_path)
//            VALUES (?, ?)
//        `)
//		if err != nil {
//			return 0, fmt.Errorf("insertVisual (prepare photo stmt): %v", err)
//		}
//		defer stmt.Close()
//
//		for _, path := range visual.Photos {
//			if _, err = stmt.Exec(visualID, path); err != nil {
//				return 0, fmt.Errorf("insertVisual (insert photo %s): %v", path, err)
//			}
//		}
//	}
//
//	if err = tx.Commit(); err != nil {
//		return 0, fmt.Errorf("insertVisual (commit): %v", err)
//	}
//
//	return visualID, nil
//}
//
//func insertStory(story Story) (int, error) {
//	sqlStmt := `
//		INSERT INTO stories (user_id, title, content) VALUES (?, ?, ?) RETURNING id;
//	`
//	var id int
//	err := DB.QueryRow(sqlStmt, story.UserID, story.Title, story.Content).Scan(&id)
//	if err != nil {
//		return 0, fmt.Errorf("insertStory: %v", err)
//	}
//	return id, nil
//}
//
//func updateVisual(visual Visual) error {
//	_, err := DB.Exec(`
//        UPDATE visuals
//        SET title = ?, description = ?
//        WHERE id = ?`,
//		visual.Title, visual.Description, visual.ID)
//	if err != nil {
//		return err
//	}
//
//	if len(visual.Photos) > 0 {
//		stmt, err := DB.Prepare(`
//       INSERT INTO visual_photos (visual_id, file_path)
//       VALUES (?, ?)`)
//		if err != nil {
//			return err
//		}
//		defer stmt.Close()
//
//		for _, path := range visual.Photos {
//			if _, err := stmt.Exec(visual.ID, path); err != nil {
//				return err
//			}
//		}
//	}
//	return nil
//}
//
//func updateStory(story Story) error {
//	sqlStmt := `
//       UPDATE stories
//       SET title = ?, content = ?
//       WHERE id = ? AND user_id = ?;
//    `
//	result, err := DB.Exec(sqlStmt, story.Title, story.Content, story.ID, story.UserID)
//	if err != nil {
//		return fmt.Errorf("updateStory: %v", err)
//	}
//
//	rowsAffected, err := result.RowsAffected()
//	if err != nil {
//		return fmt.Errorf("updateStory (rows affected): %v", err)
//	}
//	if rowsAffected == 0 {
//		return fmt.Errorf("no rows updated - either story doesn't exist or user doesn't have permission")
//	}
//
//	return nil
//}
//

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

//func sanitizeFilename(input string) string {
//	// Replace spaces with underscores
//	output := strings.ReplaceAll(input, " ", "_")
//	// Remove any other problematic characters
//	output = strings.Map(func(r rune) rune {
//		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-' {
//			return r
//		}
//		return -1
//	}, output)
//	return output
//}
//
//func deleteVisual(id int) error {
//	tx, err := DB.Begin()
//	if err != nil {
//		return err
//	}
//	defer tx.Rollback()
//
//	log.Printf("Attempt to delete visual with id '%d'", id)
//	if _, err := tx.Exec(`DELETE FROM visual_photos WHERE visual_id = ?`, id); err != nil {
//		log.Printf("Failed to delete photos with visual id '%d': %v", id, err)
//		return err
//	}
//
//	if _, err := tx.Exec(`DELETE FROM visuals WHERE id = ?`, id); err != nil {
//		log.Printf("Failed to delete visual with id '%d': %v", id, err)
//		return err
//	}
//
//	if err := tx.Commit(); err != nil {
//		log.Printf("Failed to delete visual with id '%d': %v", id, err)
//		return err
//	}
//
//	log.Printf("Successfully deleted visual with id '%d'", id)
//	return nil
//}
//
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

////	if req.Method == http.MethodPost {
////		if !loggedIn {
////			http.Error(w, "Unauthorized", http.StatusForbidden)
////			return
////		}
////
////		if req.FormValue("_method") == "DELETE" {
////			if err := deleteVisual(visual.ID); err != nil {
////				http.Error(w, "Failed to delete visual", http.StatusInternalServerError)
////				log.Printf("Error deleting visual: %v", err)
////				return
////			}
////			// Cleanup files
////			go cleanupVisualFiles(visual)
////			http.Redirect(w, req, "/visual", http.StatusSeeOther)
////			return
////		}
////
////		// Parse form (including multipart for potential new photos)
////		if err := req.ParseMultipartForm(32 << 20); err != nil {
////			http.Error(w, "Unable to parse form data", http.StatusBadRequest)
////			return
////		}
////
////		// Update basic fields
////		updatedVisual := Visual{
////			ID:          visual.ID,
////			UserID:      visual.UserID, // Preserve original owner
////			Title:       req.FormValue("title"),
////			Description: req.FormValue("description"),
////		}
////
////		// Process new file uploads if any
////		if fileHeaders := req.MultipartForm.File["photos"]; len(fileHeaders) > 0 {
////			newPaths, err := saveVisualPhotos(updatedVisual.Title, fileHeaders)
////			if err != nil {
////				http.Error(w, "Failed to save photos", http.StatusInternalServerError)
////				log.Printf("Error saving photos: %v", err)
////				return
////			}
////			updatedVisual.Photos = append(updatedVisual.Photos, newPaths...)
////		}
////
////		// Update in database
////		if err := updateVisual(updatedVisual); err != nil {
////			http.Error(w, "Failed to update visual", http.StatusInternalServerError)
////			log.Printf("Error updating visual: %v", err)
////			return
////		}
////
////		http.Redirect(w, req, req.URL.Path, http.StatusSeeOther)
////		return
////	}
//
////func cleanupVisualFiles(visual Visual) {
////    safeTitle := sanitizeFilename(visual.Title)
////    visualDir := filepath.Join(".localFSDir/serve/visual", safeTitle)
////    if err := os.RemoveAll(visualDir); err != nil {
////        log.Printf("Error cleaning up visual files: %v", err)
////    }
////}
////
