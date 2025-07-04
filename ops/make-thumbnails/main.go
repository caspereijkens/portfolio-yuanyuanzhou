package main

import (
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
)

type ThumbnailConfig struct {
	Name    string
	Width   int
	Quality int
	Crop    bool
}

var thumbnailConfigs = []ThumbnailConfig{
	{Name: "mini", Width: 40, Quality: 80, Crop: true},
	{Name: "small", Width: 150, Quality: 80},
	{Name: "medium", Width: 600, Quality: 80},
	{Name: "large", Width: 1080, Quality: 80},
}

func main() {
	visualsDirPtr := flag.String("dir", "data/serve/visuals", "The root directory of the visuals to process.")
	flag.Parse()
	visualsDir := *visualsDirPtr

	fmt.Printf("Processing visuals in: %s\n", visualsDir)

	entries, err := os.ReadDir(visualsDir)
	if err != nil {
		fmt.Printf("Error reading visuals directory: %v\n", err)
		os.Exit(1)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if the directory name is a number (visual ID)
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}

		visualPath := filepath.Join(visualsDir, entry.Name())
		fmt.Printf("\nProcessing visual directory: %s\n", visualPath)

		// Clean up existing thumbnails before generating new ones
		thumbnailsPath := filepath.Join(visualPath, "thumbnails")
		if _, err := os.Stat(thumbnailsPath); err == nil {
			fmt.Printf("  - Removing existing thumbnails directory: %s\n", thumbnailsPath)
			if err := os.RemoveAll(thumbnailsPath); err != nil {
				fmt.Printf("    - Error removing thumbnails: %v\n", err)
				continue
			}
		}

		err = filepath.Walk(visualPath, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
				return nil
			}

			fmt.Printf("  - Processing file: %s\n", info.Name())
			img, err := imaging.Open(path, imaging.AutoOrientation(true))
			if err != nil {
				fmt.Printf("    - Skipping, not a valid image: %s\n", info.Name())
				return nil // Continue with the next file
			}

			for _, config := range thumbnailConfigs {
				var thumbErr error
				if config.Crop {
					thumbErr = saveCroppedThumbnail(img, config.Width, config.Name, path, config.Quality)
				} else {
					thumbErr = saveResizedImage(img, config.Width, config.Name, path, config.Quality)
				}
				if thumbErr != nil {
					fmt.Printf("    - Error generating %s thumbnail: %v\n", config.Name, thumbErr)
				}
			}
			return nil
		})

		if err != nil {
			fmt.Printf("Error walking the path %s: %v\n", visualPath, err)
		}
	}

	fmt.Println("\nThumbnail generation completed!")
}

func saveResizedImage(img image.Image, width int, sizeName, originalImagePath string, quality int) error {
	resizedImg := imaging.Resize(img, width, 0, imaging.Lanczos)
	return saveImage(resizedImg, sizeName, originalImagePath, quality)
}

func saveCroppedThumbnail(img image.Image, size int, sizeName, originalImagePath string, quality int) error {
	thumb := imaging.Fill(img, size, size, imaging.Center, imaging.Lanczos)
	return saveImage(thumb, sizeName, originalImagePath, quality)
}

func saveImage(img image.Image, sizeName, originalImagePath string, quality int) error {
	thumbDir := filepath.Join(filepath.Dir(originalImagePath), "thumbnails", sizeName)
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return fmt.Errorf("error creating %s thumbnail directory: %v", sizeName, err)
	}
	thumbPath := filepath.Join(thumbDir, filepath.Base(originalImagePath))
	outFile, err := os.Create(thumbPath)
	if err != nil {
		return fmt.Errorf("error creating output file for %s thumbnail: %v", sizeName, err)
	}
	defer outFile.Close()

	return jpeg.Encode(outFile, img, &jpeg.Options{Quality: quality})
}