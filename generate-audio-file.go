package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
)

// AudioBatchConfig holds configuration for audio generation batching
type AudioBatchConfig struct {
	MaxConcurrent int           // Maximum number of concurrent requests
	RetryDelay    time.Duration // Delay between retries on failure
	MaxRetries    int           // Maximum number of retries per request
}

var defaultAudioBatchConfig = AudioBatchConfig{
	MaxConcurrent: 2,      // Process 2 audio requests at a time (TTS can be resource-intensive)
	RetryDelay:    8 * time.Second,
	MaxRetries:    3,
}

// Initialize a semaphore to control concurrent audio requests
var audioSemaphore chan struct{}
var once sync.Once

func init() {
	once.Do(func() {
		audioSemaphore = make(chan struct{}, defaultAudioBatchConfig.MaxConcurrent)
	})
}

// GenerateAudioFile converts article text to speech and saves it as an MP3 file
func GenerateAudioFile(content string) (string, error) {
	return GenerateAudioFileWithConfig(content, defaultAudioBatchConfig)
}

// GenerateAudioFileWithConfig allows custom batch configuration
func GenerateAudioFileWithConfig(content string, config AudioBatchConfig) (string, error) {
	var lastErr error
	
	for retry := 0; retry <= config.MaxRetries; retry++ {
		if retry > 0 {
			time.Sleep(config.RetryDelay)
		}

		// Acquire semaphore token
		audioSemaphore <- struct{}{}
		defer func() { <-audioSemaphore }()

		outputPath, err := generateAudioWithRetry(content)
		if err == nil {
			return outputPath, nil
		}

		// Handle rate limit errors specially
		if strings.Contains(err.Error(), "rate limit") {
			time.Sleep(config.RetryDelay * 2)
			lastErr = err
			continue
		}

		// For other errors, return immediately
		return "", err
	}

	return "", fmt.Errorf("max retries exceeded: %v", lastErr)
}

func generateAudioWithRetry(content string) (string, error) {
	// Append the outro message
	content = content + " I'm Daily Bot, and you're listening to Daily Scoop AI."

	apiKey := os.Getenv("OPENAI_API_KEY")
	client := openai.NewClient(apiKey)
	ctx := context.Background()

	req := openai.CreateSpeechRequest{
		Model: openai.TTSModel1,
		Input: content,
		Voice: openai.VoiceAlloy,
		ResponseFormat: openai.SpeechResponseFormatMp3,
	}

	resp, err := client.CreateSpeech(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to synthesize speech: %v", err)
	}
	defer resp.Close()

	// Create output directory if it doesn't exist
	outputDir := "media/audio"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %v", err)
	}

	// Generate unique filename using timestamp
	filename := fmt.Sprintf("news_%d.mp3", time.Now().UnixNano())
	outputPath := filepath.Join(outputDir, filename)

	// Create the output file
	out, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %v", err)
	}
	defer out.Close()

	// Copy the audio content to file
	if _, err := io.Copy(out, resp); err != nil {
		// Clean up the file if we failed to write it
		os.Remove(outputPath)
		return "", fmt.Errorf("failed to write audio file: %v", err)
	}

	return outputPath, nil
} 