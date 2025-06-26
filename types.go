package main

import (
	"time"
)

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
	UpdatedAt   time.Time `db:"updated_at"`
	Photos      []Photo
}

type Photo struct {
	ID        int       `json:"id"`
	VisualID  int       `json:"visual_id"`
	Filename  string    `json:"file_path"`
	CreatedAt time.Time `json:"created_at"`
}

type PhotoResponse struct {
	Photos     []Photo                `json:"photos"`
	Pagination map[string]interface{} `json:"pagination"`
}

type loginData struct {
	Login bool
}

type coverData struct {
	Login             bool
	OriginalCoverPath string
	LargeCoverPath    string
	MediumCoverPath   string
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

type ThumbnailConfig struct {
	Name      string
	Width     int
	Quality   int
	Crop      bool
}

type FileUploadConfig struct {
	AllowedTypes   map[string]bool
	DestinationDir string
	MaxSize        int64
	Filename       string
	Thumbnails     []ThumbnailConfig
}

type thumbnailPaths struct {
    Mini   string `json:"mini"`
    Small  string `json:"small"`
    Medium string `json:"medium"`
    Large  string `json:"large"`
}

type photoResponse struct {
	ID         int            `json:"id"`
	Filename   string         `json:"filename"`
	Thumbnails thumbnailPaths `json:"thumbnails"`
}
