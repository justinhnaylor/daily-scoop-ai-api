package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type SummarizationRequest struct {
	Content string
}

func SummarizeArticles(articles []ArticleContent) (map[string]string, error) {
	summaries := make(map[string]string)
	var mutex sync.Mutex

	batchSize := 10
	maxContentLength := 60000

	for i := 0; i < len(articles); i += batchSize {
		startBatchTime := time.Now()
		end := i + batchSize
		if end > len(articles) {
			end = len(articles)
		}

		var wg sync.WaitGroup
		errorChan := make(chan error, end-i)

		for j := i; j < end; j++ {
			wg.Add(1)
			go func(article ArticleContent) {
				defer wg.Done()
				if len(article.Content) > maxContentLength {
					log.Printf("INFO: Skipping article (too long): %s - URL: %s", article.Title, article.URL)
					return
				}

				log.Printf("DEBUG: Starting summarization for article: %s", article.Title)

				cmd := exec.Command("python3", "summarizer.py")
				stdin, err := cmd.StdinPipe()
				if err != nil {
					log.Printf("ERROR: Error creating stdin pipe for %s - URL: %s, Error: %v", article.Title, article.URL, err)
					errorChan <- err
					return
				}

				stdout, err := cmd.StdoutPipe()
				if err != nil {
					log.Printf("ERROR: Error creating stdout pipe for %s - URL: %s, Error: %v", article.Title, article.URL, err)
					errorChan <- err
					return
				}

				stderr, err := cmd.StderrPipe()
				if err != nil {
					log.Printf("ERROR: Error creating stderr pipe for %s - URL: %s, Error: %v", article.Title, article.URL, err)
					errorChan <- err
					return
				}

				if err := cmd.Start(); err != nil {
					log.Printf("ERROR: Error starting command for %s - URL: %s, Error: %v", article.Title, article.URL, err)
					errorChan <- err
					return
				}
				log.Printf("DEBUG: Processing article: %s - URL: %s", article.Title, article.URL)

				// Create a channel for Python script output
				stderrChan := make(chan string, 100)

				// Read stderr in a goroutine
				go func() {
					scanner := bufio.NewScanner(stderr)
					for scanner.Scan() {
						debugMsg := scanner.Text()
						stderrChan <- debugMsg
						
						var logMsg map[string]interface{}
						if err := json.Unmarshal([]byte(debugMsg), &logMsg); err == nil {
							if debug, ok := logMsg["debug"].(string); ok {
								log.Printf("DEBUG (Python - %s): %s", article.URL, debug)
							} else if errMsg, ok := logMsg["error"].(string); ok {
								log.Printf("ERROR (Python - %s): %s", article.URL, errMsg)
							}
						}
					}
					close(stderrChan)
				}()

				// Write content length followed by content
				fmt.Fprintf(stdin, "%d\n", len(article.Content))
				fmt.Fprint(stdin, article.Content)
				stdin.Close()

				// Read the output
				output, err := io.ReadAll(stdout)
				if err != nil {
					// Collect any error messages from stderr
					var stderrMsgs []string
					for msg := range stderrChan {
						stderrMsgs = append(stderrMsgs, msg)
					}
					errorDetail := strings.Join(stderrMsgs, "\n")
					log.Printf("ERROR: Error reading output for %s - URL: %s, Error: %v\nPython Error Details:\n%s", 
						article.Title, article.URL, err, errorDetail)
					errorChan <- fmt.Errorf("failed to read output: %v (Python errors: %s)", err, errorDetail)
					return
				}

				// Collect any error messages from stderr before checking cmd.Wait()
				var stderrMsgs []string
				for msg := range stderrChan {
					stderrMsgs = append(stderrMsgs, msg)
				}
				errorDetail := strings.Join(stderrMsgs, "\n")

				if err := cmd.Wait(); err != nil {
					log.Printf("ERROR: Command failed for %s - URL: %s, Error: %v\nPython Error Details:\n%s", 
						article.Title, article.URL, err, errorDetail)
					if errorDetail != "" {
						errorChan <- fmt.Errorf("command failed: %v (Python errors: %s)", err, errorDetail)
					} else {
						errorChan <- fmt.Errorf("command failed: %v", err)
					}
					return
				}

				// Only proceed with JSON parsing if we have output
				if len(output) == 0 {
					errorMsg := "No output received from Python script"
					if errorDetail != "" {
						errorMsg = fmt.Sprintf("%s\nPython Error Details:\n%s", errorMsg, errorDetail)
					}
					log.Printf("ERROR: %s for %s - URL: %s", errorMsg, article.Title, article.URL)
					errorChan <- fmt.Errorf(errorMsg)
					return
				}

				// Try to parse the JSON output
				var result struct {
					Success bool   `json:"success"`
					Summary string `json:"summary"`
					Error   string `json:"error"`
				}

				if err := json.Unmarshal(output, &result); err != nil {
					log.Printf("ERROR: Failed to parse JSON output for %s - URL: %s\nOutput: %s\nError: %v", 
						article.Title, article.URL, string(output), err)
					errorChan <- fmt.Errorf("failed to parse JSON output: %v (output: %s)", err, string(output))
					return
				}

				if !result.Success {
					errorMsg := fmt.Sprintf("Summarization failed: %s", result.Error)
					log.Printf("ERROR: %s for %s - URL: %s", errorMsg, article.Title, article.URL)
					errorChan <- fmt.Errorf(errorMsg)
					return
				}

				// Store the summary with mutex lock
				mutex.Lock()
				summaries[article.URL] = result.Summary
				mutex.Unlock()
				log.Printf("INFO: Successfully summarized article: %s - URL: %s", article.Title, article.URL)

			}(articles[j])
		}

		wg.Wait()
		close(errorChan)

		for err := range errorChan {
			if err != nil {
				return nil, err
			}
		}
		batchDuration := time.Since(startBatchTime)
		log.Printf("INFO: Processed batch of %d articles in %v", (end - i), batchDuration)
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