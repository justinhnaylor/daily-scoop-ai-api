package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"
)

type SummarizationRequest struct {
	Content string
}

func SummarizeArticles(articles []ArticleContent) (map[string]string, error) {
	summaries := make(map[string]string)
	var mutex sync.Mutex
	maxContentLength := 60000

	// Create a single Python process
	cmd := exec.Command("python3", "summarizer.py")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("error creating stdin pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("error creating stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("error creating stderr pipe: %v", err)
	}

	// Create a channel for Python script output
	stderrChan := make(chan string, 100)

	// Start the Python process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("error starting Python process: %v", err)
	}

	// Read stderr in a goroutine
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrChan <- scanner.Text()
		}
		close(stderrChan)
	}()

	// Process articles sequentially
	for _, article := range articles {
		if len(article.Content) > maxContentLength {
			log.Printf("INFO: Skipping article (too long): %s - URL: %s", article.Title, article.URL)
			continue
		}

		log.Printf("DEBUG: Starting summarization for article: %s", article.Title)

		// Write content length and content to stdin
		fmt.Fprintf(stdin, "%d\n", len(article.Content))
		io.WriteString(stdin, article.Content)

		// Read and process the result
		result, err := io.ReadAll(stdout)
		if err != nil {
			log.Printf("ERROR: Error reading stdout for %s - URL: %s, Error: %v", article.Title, article.URL, err)
			cmd.Process.Kill()
			break
		}

		// Process the result
		var response struct {
			Success bool   `json:"success"`
			Summary string `json:"summary"`
			Error   string `json:"error"`
		}

		if err := json.Unmarshal(result, &response); err != nil {
			log.Printf("ERROR: Error parsing JSON for %s - URL: %s, Error: %v", article.Title, article.URL, err)
			continue
		}

		if response.Success {
			mutex.Lock()
			summaries[article.URL] = response.Summary
			mutex.Unlock()
		} else {
			log.Printf("ERROR: Summarization failed for %s - URL: %s, Error: %s", article.Title, article.URL, response.Error)
		}

		// Print any debug/error messages from stderr
		select {
		case msg := <-stderrChan:
			var debugMsg struct {
				Debug string `json:"debug"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal([]byte(msg), &debugMsg); err == nil {
				if debugMsg.Debug != "" {
					log.Printf("DEBUG [%s]: %s", article.Title, debugMsg.Debug)
				}
				if debugMsg.Error != "" {
					log.Printf("ERROR [%s]: %s", article.Title, debugMsg.Error)
				}
			}
		default:
			// No messages to process
		}

		// Add a small delay between articles
		time.Sleep(1 * time.Second)
	}

	// Clean up
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		log.Printf("ERROR: Error waiting for Python process to finish: %v", err)
	}

	if len(summaries) == 0 {
		return nil, fmt.Errorf("no successful summaries generated")
	}

	return summaries, nil
}

func StartSummarizer() {
	log.Println("Starting Python summarizer pre-warming in background...")

	cmd := exec.Command("python3", "summarizer.py")
	cmd.Stderr = log.Writer()
	cmd.Stdout = log.Writer()

	if err := cmd.Start(); err != nil {
		log.Printf("Error starting pre-warming script: %v", err)
		return
	}

	log.Println("Python summarizer pre-warming started in background.")
}