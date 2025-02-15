package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log" // Using standard log package for simplicity
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// ... (SummarizationRequest, init, checkPythonDependencies, createPythonScript - remain the same) ...

func SummarizeArticles(articles []ArticleContent) (map[string]string, error) {
	summaries := make(map[string]string)
	var mutex sync.Mutex

	batchSize := 10
	maxContentLength := 60000

	pythonPath := filepath.Join(".venv", "bin", "python3")
	scriptPath := filepath.Join(os.TempDir(), "summarizer.py")

	for i := 0; i < len(articles); i += batchSize {
		startBatchTime := time.Now() // Time batch processing
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

				cmd := exec.Command(pythonPath, scriptPath)
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
				log.Printf("DEBUG: Processing article: %s - URL: %s", article.Title, article.URL) // Debug log

				fmt.Fprintf(stdin, "%d\n", len(article.Content))
				fmt.Fprint(stdin, article.Content)
				stdin.Close()

				go func() { // Goroutine to read stderr
					buf := make([]byte, 1024)
					for {
						n, err := stderr.Read(buf)
						if n > 0 {
							log.Printf("DEBUG (Python - %s): %s", article.URL, string(buf[:n])) // Python stderr output
						}
						if err != nil {
							if err != io.EOF {
								log.Printf("ERROR: Error reading stderr for %s - URL: %s, Error: %v", article.Title, article.URL, err)
							}
							break
						}
					}
				}()

				output, err := io.ReadAll(stdout)
				if err != nil {
					log.Printf("ERROR: Error reading output for %s - URL: %s, Error: %v", article.Title, article.URL, err)
					errorChan <- err
					return
				}

				if err := cmd.Wait(); err != nil {
					log.Printf("ERROR: Command failed for %s - URL: %s, Error: %v", article.Title, article.URL, err)
					errorChan <- err
					return
				}

				var result struct {
					Success bool   `json:"success"`
					Summary string `json:"summary"`
					Error   string `json:"error"`
				}

				if err := json.Unmarshal(output, &result); err != nil {
					log.Printf("ERROR: Error parsing JSON result for %s - URL: %s, Error: %v, Output: %s", article.Title, article.URL, err, string(output))
					errorChan <- err
					return
				}

				if result.Success {
					mutex.Lock()
					summaries[article.URL] = result.Summary
					mutex.Unlock()
					log.Printf("INFO: Successfully summarized article: %s - URL: %s", article.Title, article.URL)
				} else if result.Error != "" {
					log.Printf("WARNING: Summarization failed for %s - URL: %s, Error: %s", article.Title, article.URL, result.Error)
				}

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
		log.Printf("INFO: Processed batch of %d articles in %v", (end - i), batchDuration) // Log batch processing time
	}

	return summaries, nil
}