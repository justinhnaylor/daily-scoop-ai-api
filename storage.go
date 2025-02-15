package main

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/h2non/bimg"
)

const (
	supabaseProjectURL = "https://dymrplcuovidgyepquba.supabase.co"
	
	// Image dimensions
	bannerWidth  = 1920  // Standard HD width
	bannerHeight = 1080  // Standard HD height (16:9 ratio)
	thumbSize    = 500   // Thumbnail size (both width and height)
)

// MediaOptimizer handles compression of media files
type MediaOptimizer struct {
	AudioBitrate string // e.g., "128k"
}

func NewMediaOptimizer() *MediaOptimizer {
	return &MediaOptimizer{
		AudioBitrate: "128k", // decent quality for voice
	}
}

// Add this interface at the top of the file
type SubImager interface {
	SubImage(r image.Rectangle) image.Image
}

func (m *MediaOptimizer) OptimizeImage(inputPath string) (string, error) {
	// Read and validate input
	buffer, err := bimg.Read(inputPath)
	if err != nil {
		return "", fmt.Errorf("failed to read image: %v", err)
	}

	// Create output paths
	basePath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath))
	bannerPath := basePath + "_banner.webp"
	thumbnailPath := basePath + "_thumb.webp"

	// Process banner
	if err := m.createBanner(buffer, bannerPath); err != nil {
		return "", fmt.Errorf("failed to create banner: %v", err)
	}

	// Process thumbnail
	if err := m.createThumbnail(buffer, thumbnailPath); err != nil {
		return "", fmt.Errorf("failed to create thumbnail: %v", err)
	}

	return bannerPath, nil
}

func (m *MediaOptimizer) createBanner(buffer []byte, outputPath string) error {
	size, err := bimg.NewImage(buffer).Size()
	if err != nil {
		return fmt.Errorf("failed to get image dimensions: %v", err)
	}

	// Calculate resize dimensions
	widthRatio := float64(bannerWidth) / float64(size.Width)
	heightRatio := float64(bannerHeight) / float64(size.Height)
	resizeRatio := math.Max(widthRatio, heightRatio)
	resizedWidth := int(float64(size.Width) * resizeRatio)
	resizedHeight := int(float64(size.Height) * resizeRatio)

	// Resize image
	banner, err := bimg.NewImage(buffer).Process(bimg.Options{
		Width:   resizedWidth,
		Height:  resizedHeight,
		Force:   true,
		Enlarge: true,
		Type:    bimg.WEBP,
	})
	if err != nil {
		return fmt.Errorf("failed to resize banner: %v", err)
	}

	// Crop to 16:9
	resizedSize, err := bimg.NewImage(banner).Size()
	if err != nil {
		return fmt.Errorf("failed to get resized dimensions: %v", err)
	}

	x := (resizedSize.Width - bannerWidth) / 2
	y := (resizedSize.Height - bannerHeight) / 2
	banner, err = bimg.NewImage(banner).Extract(y, x, bannerWidth, bannerHeight)
	if err != nil {
		return fmt.Errorf("failed to crop banner: %v", err)
	}

	return bimg.Write(outputPath, banner)
}

func (m *MediaOptimizer) createThumbnail(buffer []byte, outputPath string) error {
	size, err := bimg.NewImage(buffer).Size()
	if err != nil {
		return fmt.Errorf("failed to get image dimensions: %v", err)
	}

	// Resize maintaining aspect ratio
	var thumb []byte
	if size.Height < size.Width {
		thumb, err = bimg.NewImage(buffer).Process(bimg.Options{
			Width:   0,
			Height:  thumbSize,
			Force:   true,
			Type:    bimg.WEBP,
		})
	} else {
		thumb, err = bimg.NewImage(buffer).Process(bimg.Options{
			Width:   thumbSize,
			Height:  0,
			Force:   true,
			Type:    bimg.WEBP,
		})
	}
	if err != nil {
		return fmt.Errorf("failed to resize thumbnail: %v", err)
	}

	// Crop to square
	resizedSize, err := bimg.NewImage(thumb).Size()
	if err != nil {
		return fmt.Errorf("failed to get resized dimensions: %v", err)
	}

	x := (resizedSize.Width - thumbSize) / 2
	y := (resizedSize.Height - thumbSize) / 2
	thumb, err = bimg.NewImage(thumb).Extract(y, x, thumbSize, thumbSize)
	if err != nil {
		return fmt.Errorf("failed to crop thumbnail: %v", err)
	}

	return bimg.Write(outputPath, thumb)
}

func (m *MediaOptimizer) OptimizeAudio(inputPath string) (string, error) {
	outputPath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".mp3"
	
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-codec:a", "libmp3lame",  // Use MP3 codec
		"-b:a", m.AudioBitrate,    // Bitrate (e.g., "128k")
		"-ar", "44100",            // Sample rate
		"-ac", "2",                // Stereo
		outputPath)
	
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to optimize audio: %v", err)
	}
	
	return outputPath, nil
}

// UploadToStorage uploads a file to Supabase storage and returns the public URL
func uploadToStorage(filePath string, bucket string) (string, error) {
	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	// Get service role key and clean it
	serviceKey := os.Getenv("SUPABASE_SERVICE_KEY")
	if serviceKey == "" {
		return "", fmt.Errorf("SUPABASE_SERVICE_KEY environment variable not set")
	}
	// Remove any quotes from the key
	serviceKey = strings.Trim(serviceKey, "\"")

	// Clean and encode bucket name
	bucket = strings.Trim(bucket, "\"")
	encodedBucket := url.PathEscape(bucket)

	// Prepare the request
	fileName := filepath.Base(filePath)
	encodedFileName := url.PathEscape(fileName)
	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", supabaseProjectURL, encodedBucket, encodedFileName)
	
	// Detect content type
	ext := filepath.Ext(filePath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+serviceKey)
	req.Header.Set("apikey", serviceKey)
	
	// Set cache control for media files
	if bucket == "images" || bucket == "audio" {
		req.Header.Set("Cache-Control", "public, max-age=31536000") // 1 year
	}

	// Print request details for debugging
	fmt.Printf("Making request to: %s\n", url)
	fmt.Printf("Authorization: Bearer %s\n", serviceKey[:10]+"...")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %v", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Get public URL
	publicUrl := fmt.Sprintf("%s/storage/v1/object/public/%s/%s", supabaseProjectURL, bucket, fileName)
	return publicUrl, nil
}

func UploadMediaAssets(assets NewsMediaAssets) (NewsMediaAssets, error) {
	var updatedAssets NewsMediaAssets
	optimizer := NewMediaOptimizer()

	// Upload image
	if assets.ImagePath != "" {
		bannerPath, err := optimizer.OptimizeImage(assets.ImagePath)
		if err != nil {
			return updatedAssets, fmt.Errorf("failed to optimize image: %v", err)
		}
		
		// Get the thumbnail path from the banner path
		basePath := strings.TrimSuffix(bannerPath, "_banner.webp")
		thumbnailPath := basePath + "_thumb.webp"
		
		// Upload banner
		bannerURL, err := uploadToStorage(bannerPath, "images")
		if err != nil {
			return updatedAssets, fmt.Errorf("failed to upload banner image: %v", err)
		}
		updatedAssets.ImagePath = bannerURL

		// Upload thumbnail
		thumbnailURL, err := uploadToStorage(thumbnailPath, "images")
		if err != nil {
			return updatedAssets, fmt.Errorf("failed to upload thumbnail: %v", err)
		}
		updatedAssets.ThumbnailPath = thumbnailURL

		// Clean up local files
		os.Remove(assets.ImagePath)
		os.Remove(bannerPath)
		os.Remove(thumbnailPath)
	}

	// Upload audio
	if assets.AudioPath != "" {
		optimizedPath, err := optimizer.OptimizeAudio(assets.AudioPath)
		if err != nil {
			return updatedAssets, fmt.Errorf("failed to optimize audio: %v", err)
		}
		
		audioURL, err := uploadToStorage(optimizedPath, "audio")
		if err != nil {
			return updatedAssets, fmt.Errorf("failed to upload audio: %v", err)
		}
		updatedAssets.AudioPath = audioURL

		// Clean up local files
		os.Remove(assets.AudioPath)
		os.Remove(optimizedPath)
	}

	return updatedAssets, nil
} 

