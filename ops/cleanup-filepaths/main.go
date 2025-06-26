package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type VisualPhoto struct {
	ID       int
	FilePath string
}

func main() {
	dbPath := "./data/sqlite.DB"
	fmt.Printf("Connecting to database: %s\n", dbPath)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("FATAL: Could not open database: %v", err)
	}
	defer db.Close()

	// Begin a transaction
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("FATAL: Could not begin transaction: %v", err)
	}

	// 1. Fetch all photos
	rows, err := tx.Query("SELECT id, file_path FROM visual_photos")
	if err != nil {
		tx.Rollback()
		log.Fatalf("FATAL: Could not query visual_photos table: %v", err)
	}
	defer rows.Close()

	var photosToUpdate []VisualPhoto
	for rows.Next() {
		var p VisualPhoto
		if err := rows.Scan(&p.ID, &p.FilePath); err != nil {
			tx.Rollback()
			log.Fatalf("FATAL: Could not scan row: %v", err)
		}
		// Check if the path needs cleaning
		if filepath.Base(p.FilePath) != p.FilePath {
			photosToUpdate = append(photosToUpdate, p)
		}
	}

	if len(photosToUpdate) == 0 {
		fmt.Println("Database is already clean. No filepaths to update.")
		os.Exit(0)
	}

	fmt.Printf("Found %d photo filepaths to clean up. Starting update...\n", len(photosToUpdate))

	// 2. Prepare the update statement
	stmt, err := tx.Prepare("UPDATE visual_photos SET file_path = ? WHERE id = ?")
	if err != nil {
		tx.Rollback()
		log.Fatalf("FATAL: Could not prepare update statement: %v", err)
	}
	defer stmt.Close()

	// 3. Execute the updates
	for _, p := range photosToUpdate {
		newFilePath := filepath.Base(p.FilePath)
		fmt.Printf("  - Updating photo ID %d: \"%s\" -> \"%s\"\n", p.ID, p.FilePath, newFilePath)
		if _, err := stmt.Exec(newFilePath, p.ID); err != nil {
			tx.Rollback()
			log.Fatalf("FATAL: Failed to update photo with ID %d: %v", p.ID, err)
		}
	}

	// 4. Commit the transaction
	if err := tx.Commit(); err != nil {
		log.Fatalf("FATAL: Could not commit transaction: %v", err)
	}

	fmt.Println("\nDatabase cleanup successful!")
}
