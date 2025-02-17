package main

import (
	"log"
	"time"
)

type TrendScheduler struct {
    stopChan chan struct{}
}

func NewTrendScheduler() *TrendScheduler {
    return &TrendScheduler{
        stopChan: make(chan struct{}),
    }
}

func (s *TrendScheduler) Start() {
    // Start daily trends (runs at 8 AM local time)
    go s.scheduleDailyTrends()
    
    // Start recent trends (runs every 2 hours)
    go s.scheduleRecentTrends()
}

func (s *TrendScheduler) Stop() {
    close(s.stopChan)
}

func (s *TrendScheduler) scheduleDailyTrends() {
    for {
        now := time.Now()
        next := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location())
        if now.After(next) {
            next = next.Add(24 * time.Hour)
        }
        
        select {
        case <-time.After(time.Until(next)):
            log.Printf("Running daily trends fetch at %v", time.Now())
            topics, err := GetTrendingKeywordsWithMode("daily")
            if err != nil {
                log.Printf("Error fetching daily trends: %v", err)
                continue
            }
            // Process the topics
            processTopics(topics, "daily")
            
        case <-s.stopChan:
            return
        }
    }
}

func (s *TrendScheduler) scheduleRecentTrends() {
    ticker := time.NewTicker(2 * time.Hour)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            log.Printf("Running recent trends fetch at %v", time.Now())
            topics, err := GetTrendingKeywordsWithMode("recent")
            if err != nil {
                log.Printf("Error fetching recent trends: %v", err)
                continue
            }
            // Process the topics
            processTopics(topics, "recent")
            
        case <-s.stopChan:
            return
        }
    }
}

func processTopics(topics []TrendingTopic, mode string) {
    log.Printf("Processing %s trends with %d topics", mode, len(topics))

    // Get search results
    searchResults, err := GetSearchResults(topics)
    if err != nil {
        log.Printf("Error getting search results for %s trends: %v", mode, err)
        return
    }

    // Create a map to store article data by keyword
    articleDataMap := make(map[string]ArticleData)

    // Scrape articles from search results
    articles, err := ScrapeArticles(searchResults)
    if err != nil {
        log.Printf("Error scraping articles for %s trends: %v", mode, err)
        return
    }

    // Organize articles by keyword
    for _, result := range searchResults {
        articleDataMap[result.Keyword] = ArticleData{
            Keyword:   result.Keyword,
            Articles:  filterArticlesByURLs(articles, result.URLs),
            Summaries: make(map[string]string),
        }
    }

    // Process each keyword's articles
    for keyword, data := range articleDataMap {
        // Summarize the articles
        summaries, err := SummarizeArticles(data.Articles)
        if err != nil {
            log.Printf("[%s trends] Error summarizing articles for %s: %v", mode, keyword, err)
            continue
        }
        data.Summaries = summaries

        // Generate comprehensive article
        article, err := GenerateArticleFromSummaries(
            keyword,
            data.Summaries,
            searchResults[0].URLs, // Using first result's URLs
        )
        if err != nil {
            log.Printf("[%s trends] Error generating article for %s: %v", mode, keyword, err)
            continue
        }

        // Generate media assets
        mediaAssets, imageSuccess, err := GenerateMediaAssets(*article)
        if err != nil {
            log.Printf("[%s trends] Error generating media assets for %s: %v", mode, keyword, err)
            continue
        }

        // Upload media assets
        uploadedAssets, err := UploadMediaAssets(mediaAssets)
        if err != nil {
            log.Printf("[%s trends] Error uploading media assets for %s: %v", mode, keyword, err)
            continue
        }

        // Save to database
        savedArticle, err := dbClient.SaveArticle(article, uploadedAssets, imageSuccess)
        if err != nil {
            log.Printf("[%s trends] Error saving article to database for %s: %v", mode, keyword, err)
            continue
        }

        log.Printf("[%s trends] Successfully processed and saved article: %s (ID: %s)", 
            mode, savedArticle.Title, savedArticle.ID)
    }
} 