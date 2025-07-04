package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func configDatabase() error {
	createStatements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL UNIQUE,
			password_digest BLOB NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS stories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(title)
		);
		CREATE INDEX IF NOT EXISTS idx_stories_created_at ON stories(created_at);`,
		`CREATE TABLE IF NOT EXISTS visuals (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_visuals_created_at ON visuals(created_at);`,
		`CREATE TABLE IF NOT EXISTS visual_photos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			visual_id INTEGER NOT NULL,
			file_path TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (visual_id) REFERENCES visuals(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_visual_photos_visual_id ON visual_photos(visual_id);`,
		`CREATE TABLE IF NOT EXISTS info (
			singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
			content TEXT NOT NULL,
			last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		INSERT OR IGNORE INTO info (singleton, content) VALUES (1, 'Welcome to my Website');`,
		`CREATE TABLE IF NOT EXISTS covers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_path TEXT NOT NULL UNIQUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS portfolios (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_path TEXT NOT NULL UNIQUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, stmt := range createStatements {
		if _, err := DB.Exec(stmt); err != nil {
			log.Printf("configDatabase: %q: %v\n", stmt, err)
			return err
		}
	}

	if err := ensureDefaultExists("covers", "file_path", "cover.png"); err != nil {
		return err
	}
	if err := ensureDefaultExists("portfolios", "file_path", "portfolios/portfolio.pdf"); err != nil {
		return err
	}

	return nil
}

func ensureDefaultExists(table, column, value string) error {
	var count int
	err := DB.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check %s count: %w", table, err)
	}
	if count == 0 {
		_, err = DB.Exec(fmt.Sprintf(`INSERT INTO %s (%s) VALUES (?)`, table, column), value)
		if err != nil {
			return fmt.Errorf("failed to insert into %s: %w", table, err)
		}
	}
	return nil
}

func getLatestCoverFilename() (string, error) {
	var filePath string
	err := DB.QueryRow("SELECT file_path FROM covers ORDER BY created_at DESC LIMIT 1").Scan(&filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get latest cover: %w", err)
	}
	return filePath, nil
}

func getInfo() (Info, error) {
	const query = `
    SELECT content 
    FROM info 
    WHERE singleton = 1
    `

	var info Info

	err := DB.QueryRow(query).Scan(&info.Content)
	if err != nil {
		return Info{}, fmt.Errorf("failed to get info: %w", err)
	}

	return info, nil
}

func updateInfo(info Info) error {
	sqlStmt := `
      UPDATE info 
      SET content = ?, last_updated = CURRENT_TIMESTAMP
      WHERE singleton = 1;
    `
	result, err := DB.Exec(sqlStmt, info.Content)
	if err != nil {
		return fmt.Errorf("updateInfo: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("updateInfo (rows affected): %v", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("no rows updated - either story doesn't exist or user doesn't have permission")
	}

	return nil
}

func getStories(id ...int) ([]Story, error) {
	var query string
	var args []any

	query = "SELECT id, title, content, created_at FROM stories"

	if len(id) > 0 {
		query += " WHERE id = ?"
		args = append(args, id[0])
	}

	query += " ORDER BY created_at DESC;"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stories []Story
	var timestamp time.Time

	for rows.Next() {
		var t Story
		if err := rows.Scan(&t.ID, &t.Title, &t.Content, &timestamp); err != nil {
			return nil, err
		}
		t.CreatedAt = timestamp
		stories = append(stories, t)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return stories, nil
}

func insertStory(story Story) (int, error) {
	sqlStmt := `
		INSERT INTO stories (title, content) VALUES (?, ?) RETURNING id;
	`
	var id int
	err := DB.QueryRow(sqlStmt, story.Title, story.Content).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insertStory: %v", err)
	}
	return id, nil
}

func updateStory(story Story) error {
	sqlStmt := `
       UPDATE stories
       SET title = ?, content = ?
       WHERE id = ?;
    `
	result, err := DB.Exec(sqlStmt, story.Title, story.Content, story.ID)
	if err != nil {
		return fmt.Errorf("updateStory: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("updateStory (rows affected): %v", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("no rows updated - either story doesn't exist or user doesn't have permission")
	}

	return nil
}

func getLatestPortfolioPath() (string, error) {
	var filePath string
	err := DB.QueryRow("SELECT file_path FROM portfolios ORDER BY created_at DESC LIMIT 1").Scan(&filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get latest portfolio: %w", err)
	}
	return filePath, nil
}

func getVisuals(id ...int) ([]Visual, error) {
	query := "SELECT id, title, description, created_at, updated_at FROM visuals"
	var args []any

	if len(id) > 0 {
		query += " WHERE id = ?"
		args = append(args, id[0])
	}
	query += " ORDER BY created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("getVisuals: %w", err)
	}
	defer rows.Close()

	var visuals []Visual
	for rows.Next() {
		var v Visual
		err := rows.Scan(&v.ID, &v.Title, &v.Description, &v.CreatedAt, &v.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("getVisuals: %w", err)
		}
		visuals = append(visuals, v)
	}

	return visuals, nil
}

func getVisualByID(id int) (*Visual, error) {
	visuals, err := getVisuals(id)
	if err != nil {
		return nil, fmt.Errorf("getVisualByID: %w", err)
	}
	if len(visuals) == 0 {
		return nil, sql.ErrNoRows
	}
	return &visuals[0], nil
}

func updateVisual(visual Visual) error {
	_, err := DB.Exec(`
      UPDATE visuals
      SET title = ?, description = ?
      WHERE id = ?`,
		visual.Title, visual.Description, visual.ID)
	if err != nil {
		return err
	}
	return nil
}

func deleteVisual(id int) error {
	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("deleteVisual (begin tx): %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p) // re-panic after rollback
		} else if err != nil {
			tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`DELETE FROM visual_photos WHERE visual_id = ?`, id); err != nil {
		return fmt.Errorf("deleteVisual (delete photos): %w", err)
	}

	if _, err = tx.Exec(`DELETE FROM visuals WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleteVisual (delete visual): %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("deleteVisual (commit tx): %w", err)
	}

	log.Printf("Successfully deleted visual with id '%d'", id)
	return nil
}

func insertVisual(visual Visual) (int, error) {
	result, err := DB.Exec(`INSERT INTO visuals (title, description) VALUES (?, ?)`, visual.Title, visual.Description)
	if err != nil {
		return 0, fmt.Errorf("insertVisual (insert visual): %v", err)
	}

	visualID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insertVisual (get ID): %v", err)
	}
	return int(visualID), nil
}

func getPhotosByVisualID(visualID, offset, limit int) ([]Photo, int, error) {
	// Get total count
	var totalCount int
	err := DB.QueryRow("SELECT COUNT(*) FROM visual_photos WHERE visual_id = ?", visualID).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("getPhotosByVisualID count: %w", err)
	}

	query := `
        SELECT id, visual_id, file_path, created_at 
        FROM visual_photos 
        WHERE visual_id = ? 
        ORDER BY created_at DESC, id DESC
    `
	args := []interface{}{visualID}

	if limit >= 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("getPhotosByVisualID query: %w", err)
	}
	defer rows.Close()

	var photos []Photo
	if limit > 0 {
		photos = make([]Photo, 0, limit)
	}

	for rows.Next() {
		var p Photo
		if err := rows.Scan(&p.ID, &p.VisualID, &p.Filename, &p.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("getPhotosByVisualID scan: %w", err)
		}
		photos = append(photos, p)
	}

	return photos, totalCount, nil
}

func getPhotoByID(id int) (*Photo, error) {
	query := "SELECT id, visual_id, file_path, created_at FROM visual_photos WHERE id = ?"
	row := DB.QueryRow(query, id)

	var p Photo
	err := row.Scan(&p.ID, &p.VisualID, &p.Filename, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("getPhotoByID: %w", err)
	}

	return &p, nil
}

func deletePhoto(id int) error {
	_, err := DB.Exec("DELETE FROM visual_photos WHERE id = ?", id)
	return err
}

func insertPhotos(visualID int, filenames []string) error {
	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("insertPhotos begin tx: %w", err)
	}

	stmt, err := tx.Prepare("INSERT INTO visual_photos (visual_id, file_path) VALUES (?, ?)")
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("insertPhotos prepare: %w", err)
	}
	defer stmt.Close()

	for _, filename := range filenames {
		_, err = stmt.Exec(visualID, filename)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("insertPhotos exec: %w", err)
		}
	}

	return tx.Commit()
}

func getCredentials(email string) (*int, []byte, error) {
	var userId int
	var passwordDigest []byte
	err := DB.QueryRow("SELECT id, password_digest FROM users WHERE email=?;", email).Scan(&userId, &passwordDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, fmt.Errorf("user not found")
	}
	if passwordDigest == nil {
		log.Println("getCredentials: password digest not found")
		return nil, nil, errors.New("password digest not found")
	}
	return &userId, passwordDigest, nil
}
