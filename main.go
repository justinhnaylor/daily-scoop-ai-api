// main.go
package main

import (
	"flag"
	"log"
	"time"

	"github.com/joho/godotenv"
	"github.com/playwright-community/playwright-go"
)

type ArticleData struct {
    Keyword   string
    Articles  []ArticleContent
    Summaries map[string]string
}

type ArticleContent struct {
    URL     string
    Title   string
    Content string
}

func main() {
	// Parse command line flags
	mode := flag.String("mode", "", "Mode to run: 'daily' or 'recent'")
	flag.Parse()

	if *mode == "" {
		log.Fatal("Mode is required: use -mode=daily or -mode=recent")
	}

	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
	}

	// Initialize database
	if err := initDB(); err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}

	go StartSummarizer()

	time.Sleep(2 * time.Second)

	// Install Playwright browsers
	if err := playwright.Install(); err != nil {
		log.Fatalf("Error installing playwright: %v", err)
	}

	// Run once for the specified mode
	log.Printf("Starting trend fetch for mode: %s", *mode)
	topics, err := GetTrendingKeywordsWithMode(*mode)
	if err != nil {
		log.Fatalf("Error fetching %s trends: %v", *mode, err)
	}

	// Process the topics
	processTopics(topics, *mode)
	log.Printf("Completed trend fetch for mode: %s", *mode)
}

// filterArticlesByURLs returns only the articles whose URLs are in the provided URLs slice
func filterArticlesByURLs(articles []ArticleContent, urls []string) []ArticleContent {
    urlSet := make(map[string]bool)
    for _, url := range urls {
        urlSet[url] = true
    }
    
    var filtered []ArticleContent
    for _, article := range articles {
        if urlSet[article.URL] {
            filtered = append(filtered, article)
        }
    }
    return filtered
}