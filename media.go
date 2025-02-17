package main

import (
	"fmt"
)

// GenerateMediaAssets creates audio and image files for a news article
func GenerateMediaAssets(article GeneratedArticle) (NewsMediaAssets, bool, error) {
	assets := NewsMediaAssets{}
	imageSuccess := true

	// Generate audio file using text-to-speech (assuming you have this function)
	audioPath, err := GenerateAudioFile(article.Article)
	if err != nil {
		return assets, imageSuccess, fmt.Errorf("failed to generate audio: %v", err)
	}
	assets.AudioPath = audioPath

	// Generate and save the image using GetNewsImage (which internally uses Gemini Flash 2)
	imagePath, err := GetNewsImage(article)
	if err != nil {
		fmt.Printf("Warning: Failed to generate image: %v\n", err)
		imageSuccess = false
	} else {
		assets.ImagePath = imagePath
	}

	return assets, imageSuccess, nil
}