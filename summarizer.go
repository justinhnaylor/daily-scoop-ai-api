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

	// Process articles sequentially instead of in batches
	for _, article := range articles {
		if len(article.Content) > maxContentLength {
			log.Printf("INFO: Skipping article (too long): %s - URL: %s", article.Title, article.URL)
			continue
		}

		log.Printf("DEBUG: Starting summarization for article: %s", article.Title)

		cmd := exec.Command("python3", "summarizer.py")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			log.Printf("ERROR: Error creating stdin pipe for %s - URL: %s, Error: %v", article.Title, article.URL, err)
			continue
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("ERROR: Error creating stdout pipe for %s - URL: %s, Error: %v", article.Title, article.URL, err)
			continue
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			log.Printf("ERROR: Error creating stderr pipe for %s - URL: %s, Error: %v", article.Title, article.URL, err)
			continue
		}

		if err := cmd.Start(); err != nil {
			log.Printf("ERROR: Error starting command for %s - URL: %s, Error: %v", article.Title, article.URL, err)
			continue
		}

		// Create a channel for Python script output
		stderrChan := make(chan string, 100)

		// Read stderr in a goroutine
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				stderrChan <- scanner.Text()
			}
			close(stderrChan)
		}()

		// Write content length and content to stdin
		fmt.Fprintf(stdin, "%d\n", len(article.Content))
		io.WriteString(stdin, article.Content)
		stdin.Close()

		// Read and process the result
		result, err := io.ReadAll(stdout)
		if err != nil {
			log.Printf("ERROR: Error reading stdout for %s - URL: %s, Error: %v", article.Title, article.URL, err)
			cmd.Process.Kill()
			continue
		}

		// Wait for the command to complete
		if err := cmd.Wait(); err != nil {
			log.Printf("ERROR: Command failed for %s - URL: %s, Error: %v", article.Title, article.URL, err)
			continue
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
		for msg := range stderrChan {
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
		}

		// Add a small delay between articles to ensure resources are freed
		time.Sleep(1 * time.Second)
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