package main

import (
	"database/sql"
	"log"
	_ "github.com/mattn/go-sqlite3"
	"fmt"
)

var DB *sql.DB

func configDatabase() error {
	createUserTable := `
    CREATE TABLE IF NOT EXISTS users (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        email story NOT NULL UNIQUE,
        password_digest BLOB NOT NULL
    );
    `
	_, err := DB.Exec(createUserTable)
	if err != nil {
		log.Printf("configDatabase: %q: %s\n", err, createUserTable)
		return err
	}

	createStoryTable := `
    CREATE TABLE IF NOT EXISTS stories (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        user_id INTEGER NOT NULL,
        title TEXT NOT NULL,
        content TEXT NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (user_id) REFERENCES users(id),
        UNIQUE(user_id, title)
    );
    
    CREATE INDEX IF NOT EXISTS idx_stories_user_id ON stories(user_id);
    CREATE INDEX IF NOT EXISTS idx_stories_created_at ON stories(created_at);
    `
	_, err = DB.Exec(createStoryTable)
	if err != nil {
		log.Printf("configDatabase: %q: %s\n", err, createStoryTable)
		return err
	}

	createVisualTable := `
    CREATE TABLE IF NOT EXISTS visuals (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        user_id INTEGER NOT NULL,
        title story NOT NULL,
        description story NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (user_id) REFERENCES users(id)
    );

    CREATE INDEX IF NOT EXISTS idx_visuals_user_id ON visuals(user_id);
    CREATE INDEX IF NOT EXISTS idx_visuals_created_at ON visuals(created_at);
    `
	_, err = DB.Exec(createVisualTable)
	if err != nil {
		log.Printf("configDatabase: %q: %s\n", err, createVisualTable)
		return err
	}

	createVisualPhotosTable := `
    CREATE TABLE IF NOT EXISTS visual_photos (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        visual_id INTEGER NOT NULL,
        file_path story NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (visual_id) REFERENCES visuals(id) ON DELETE CASCADE
    );

    CREATE INDEX IF NOT EXISTS idx_visual_photos_visual_id ON visual_photos(visual_id);
    `
	_, err = DB.Exec(createVisualPhotosTable)
	if err != nil {
		log.Printf("configDatabase: %q: %s\n", err, createVisualPhotosTable)
		return err
	}

	createInfoTable := `
	  CREATE TABLE IF NOT EXISTS info (
        singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
        
        content TEXT NOT NULL,
        last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
    
    -- Initialize the single row
    INSERT OR IGNORE INTO info (singleton, content) 
    VALUES (1, 'Welcome to my Website');
	`
	_, err = DB.Exec(createInfoTable)
	if err != nil {
		log.Printf("configDatabase: %q: %s\n", err, createInfoTable)
		return err
	}

	createCoversTable := `
    CREATE TABLE IF NOT EXISTS covers (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        file_path TEXT NOT NULL UNIQUE,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
    `
	_, err = DB.Exec(createCoversTable)
	if err != nil {
		log.Printf("configDatabase: %q: %s\n", err, createCoversTable)
		return err
	}

  var count int
  err = DB.QueryRow("SELECT COUNT(*) FROM covers").Scan(&count)
  if err != nil {
      return fmt.Errorf("failed to check covers count: %w", err)
  }

  if count == 0 {
      defaultPath := "covers/cover.png"
      _, err = DB.Exec(
          "INSERT INTO covers (file_path) VALUES (?)",
          defaultPath,
      )
      if err != nil {
          return fmt.Errorf("failed to insert default cover: %w", err)
      }
  }
	return nil
}

func getLatestCoverPath() (string, error) {
	var filePath string
	err := DB.QueryRow("SELECT file_path FROM covers ORDER BY created_at DESC LIMIT 1").Scan(&filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get latest cover: %w", err)
	}
	return filePath, nil
}
