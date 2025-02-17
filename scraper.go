package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Create a custom error type for scraping failures
type ScrapingError struct {
	FailedURLs map[string]error
	Articles   []ArticleContent
}

func (e *ScrapingError) Error() string {
	return fmt.Sprintf("failed to scrape %d URLs", len(e.FailedURLs))
}

func ScrapeArticles(searchResults []SearchResult) ([]ArticleContent, error) {
	logError := func(url string, err error, context string) {
		fmt.Printf("[%s] Error scraping %s (%s): %v\n",
			time.Now().Format("2006/01/02 15:04:05"),
			url,
			context,
			err)
	}

	var articles []ArticleContent
	failedURLs := make(map[string]error)
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
			DisableKeepAlives:   false,
		},
	}

	skipDomains := []string{
		"instagram.com",
		"facebook.com",
		"twitter.com",
		"x.com",
		"tiktok.com",
		"youtube.com",
		"reddit.com",
	}

	totalURLs := 0
	successCount := 0

	// Channels for communication between goroutines and main function
	articleChan := make(chan ArticleContent, 100) // Buffered channel for articles
	errorChan := make(chan error, 100)           // Buffered channel for errors
	var wg sync.WaitGroup

	for _, result := range searchResults {
		fmt.Printf("[%s] Processing articles for keyword: %s\n",
			time.Now().Format("2006/01/02 15:04:05"),
			result.Keyword)

		for _, url := range result.URLs {
			totalURLs++
			// Skip non-HTML URLs and social media (same as before)
			if !strings.HasPrefix(url, "http") {
				fmt.Printf("Skipping non-HTTP URL: %s\n", url)
				continue
			}
			shouldSkip := false
			for _, domain := range skipDomains {
				if strings.Contains(url, domain) {
					fmt.Printf("Skipping social media URL: %s\n", url)
					shouldSkip = true
					break
				}
			}
			if shouldSkip {
				continue
			}

			wg.Add(1)
			go func(url string) { // Start a goroutine for each URL
				defer wg.Done()

				var success bool
				var lastError error
				for attempts := 0; attempts < 3; attempts++ {
					if attempts > 0 {
						fmt.Printf("[%s] Retry attempt %d for %s\n",
							time.Now().Format("2006/01/02 15:04:05"),
							attempts+1,
							url)
						time.Sleep(time.Duration(attempts) * time.Second)
					}

					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
					if err != nil {
						lastError = fmt.Errorf("request creation failed: %v", err)
						logError(url, err, "creating request")
						cancel()
						continue
					}
					req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
					req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
					req.Header.Set("Accept-Language", "en-US,en;q=0.5")
					req.Header.Set("Connection", "keep-alive")

					resp, err := client.Do(req)
					if err != nil {
						lastError = fmt.Errorf("request failed: %v", err)
						logError(url, err, "making request")
						cancel()
						continue
					}

					processCtx, processCancel := context.WithTimeout(context.Background(), 20*time.Second)

					success = func() bool {
						defer resp.Body.Close()
						defer processCancel()

						select {
						case <-processCtx.Done():
							lastError = fmt.Errorf("processing timeout")
							logError(url, lastError, "processing timeout")
							return false
						default:
							if resp.StatusCode != http.StatusOK {
								lastError = fmt.Errorf("status code %d", resp.StatusCode)
								logError(url, lastError, "status code check")
								return false
							}

							contentType := resp.Header.Get("Content-Type")
							if !strings.Contains(strings.ToLower(contentType), "text/html") {
								lastError = fmt.Errorf("invalid content type: %s", contentType)
								logError(url, lastError, "content type check")
								return false
							}

							bodyReader := io.LimitReader(resp.Body, 10*1024*1024) // 10MB limit
							body, err := io.ReadAll(bodyReader)
							if err != nil {
								lastError = fmt.Errorf("error reading body: %v", err)
								logError(url, err, "reading body")
								return false
							}

							doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
							if err != nil {
								lastError = fmt.Errorf("error parsing HTML: %v", err)
								logError(url, err, "parsing HTML")
								return false
							}

							title := doc.Find("title").Text()
							title = cleanText(title)

							doc.Find("script").Remove()
							doc.Find("style").Remove()
							doc.Find("nav").Remove()
							doc.Find("header").Remove()
							doc.Find("footer").Remove()
							doc.Find("iframe").Remove()
							doc.Find("noscript").Remove()

							var content string
							mainContent := doc.Find("article, [role='main'], .main-content, #main-content, .post-content, .article-content, .entry-content")
							if mainContent.Length() > 0 {
								content = mainContent.Text()
							} else {
								content = doc.Find("body").Text()
							}
							content = cleanText(content)

							if content == "" {
								lastError = fmt.Errorf("no content extracted")
								logError(url, lastError, "content extraction")
								return false
							}
							if len(content) < 100 {
								lastError = fmt.Errorf("content too short (length: %d)", len(content))
								logError(url, lastError, "content validation")
								return false
							}

							articleChan <- ArticleContent{ // Send article to channel
								URL:     url,
								Title:   title,
								Content: content,
							}
							return true
						}
					}()
					cancel()
					if success {
						return // Break retry loop on success
					}
				}
				if !success {
					failedURLs[url] = lastError
					errorChan <- fmt.Errorf("scraping failed for %s after multiple retries: %v", url, lastError) // Send error to channel
					logError(url, lastError, "final failure after all attempts")
					fmt.Printf("[%s] Continuing to next URL despite failure\n",
						time.Now().Format("2006/01/02 15:04:05"))
				}
			}(url)
		}
	}

	// Start a goroutine to collect articles from the channel
	go func() {
		for article := range articleChan {
			articles = append(articles, article)
			successCount++
			fmt.Printf("[%s] Progress: %d/%d URLs successfully scraped\n",
				time.Now().Format("2006/01/02 15:04:05"),
				successCount,
				totalURLs)
		}
	}()

	// Wait for all scraping goroutines to complete
	wg.Wait()
	close(articleChan) // Close article channel to signal no more articles
	close(errorChan)   // Close error channel

	// Collect errors from error channel (optional, for consolidated error reporting)
	var consolidatedError error
	for err := range errorChan {
		if consolidatedError == nil {
			consolidatedError = err
		} else {
			consolidatedError = fmt.Errorf("%v\n%w", consolidatedError, err) // Chain errors
		}
	}

	// Enhanced summary at the end (same as before)
	fmt.Printf("\n[%s] Final Scraping Summary:\n", time.Now().Format("2006/01/02 15:04:05"))
	fmt.Printf("- Total URLs processed: %d\n", totalURLs)
	fmt.Printf("- Successfully scraped: %d articles\n", successCount)
	fmt.Printf("- Failed URLs: %d\n", len(failedURLs))
	if len(failedURLs) > 0 {
		fmt.Println("Failed URLs and reasons:")
		for url, err := range failedURLs {
			fmt.Printf("  - %s: %v\n", url, err)
		}
	}

	// Error handling logic (similar to before, but consider consolidatedError)
	if len(articles) == 0 {
		if len(failedURLs) > 0 {
			return nil, &ScrapingError{
				FailedURLs: failedURLs,
			}
		}
		return nil, fmt.Errorf("no articles were successfully scraped")
	}

	return articles, nil
}

// cleanText removes extra whitespace and normalizes text (same as before)
func cleanText(text string) string {
	// Common phrases to remove (case insensitive)
	boilerplate := []string{
		"accept cookies",
		"cookie policy",
		"privacy policy",
		"terms of service",
		"terms and conditions",
		"all rights reserved",
		"subscribe to our newsletter",
		"sign up for our newsletter",
		"share this article",
		"follow us on",
		"advertisement",
		"sponsored content",
	}

	// Remove lines containing boilerplate text
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	for _, line := range lines {
		shouldKeep := true
		lowerLine := strings.ToLower(line)
		
		// Skip empty or very short lines
		if len(strings.TrimSpace(line)) < 4 {
			continue
		}
		
		// Skip lines with boilerplate content
		for _, phrase := range boilerplate {
			if strings.Contains(lowerLine, phrase) {
				shouldKeep = false
				break
			}
		}
		
		// Skip lines that are likely navigation items (short phrases with links)
		if len(strings.Fields(line)) <= 3 && strings.Contains(line, "›") {
			continue
		}
		
		if shouldKeep {
			cleanedLines = append(cleanedLines, line)
		}
	}
	
	// Rejoin the text and normalize whitespace
	text = strings.Join(cleanedLines, " ")
	text = strings.Join(strings.Fields(text), " ")
	
	// Remove repeated punctuation
	text = regexp.MustCompile(`([.!?])\s*[$1]+`).ReplaceAllString(text, "$1")
	
	// Remove URLs
	text = regexp.MustCompile(`https?://\S+`).ReplaceAllString(text, "")
	
	return strings.TrimSpace(text)
}