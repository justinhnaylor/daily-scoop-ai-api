package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

type SearchResult struct {
	Keyword string   `json:"keyword"`
	URLs    []string `json:"urls"`
}

type GoogleSearchResponse struct {
	Items []struct {
		Link string `json:"link"`
	} `json:"items"`
}

// GetSearchResults takes trending topics and returns search results for each keyword
func GetSearchResults(topics []TrendingTopic) ([]SearchResult, error) {
	fmt.Printf("Processing %d topics\n", len(topics))

	apiKey := os.Getenv("GOOGLE_API_KEY")
	searchEngineID := os.Getenv("GOOGLE_SEARCH_ENGINE_ID")

	if apiKey == "" || searchEngineID == "" {
		return nil, fmt.Errorf("GOOGLE_API_KEY and GOOGLE_SEARCH_ENGINE_ID must be set")
	}

	var results []SearchResult

	for _, topic := range topics {
		fmt.Printf("Searching for keyword: %s\n", topic.Keyword)

		// Build the Google Custom Search API URL
		baseURL := "https://www.googleapis.com/customsearch/v1"
		params := url.Values{}
		params.Add("key", apiKey)
		params.Add("cx", searchEngineID)
		params.Add("q", topic.Keyword + " news")
		params.Add("num", "3")
		params.Add("dateRestrict", "d1") 
		params.Add("orderBy", "relevance")

		// Make the request
		resp, err := http.Get(baseURL + "?" + params.Encode())
		if err != nil {
			fmt.Printf("Error searching for %s: %v\n", topic.Keyword, err)
			continue
		}
		defer resp.Body.Close()

		// Check response status code
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("API error for %s: Status %d, Body: %s\n", topic.Keyword, resp.StatusCode, string(body))
			continue
		}

		// Parse the response
		var searchResp GoogleSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
			fmt.Printf("Error parsing response for %s: %v\n", topic.Keyword, err)
			continue
		}

		// Add debug logging for response
		fmt.Printf("Raw response for %s: %+v\n", topic.Keyword, searchResp)

		// Extract URLs
		var urls []string
		for _, item := range searchResp.Items {
			urls = append(urls, item.Link)
		}

		// Add debug logging
		fmt.Printf("Found %d URLs for %s\n", len(urls), topic.Keyword)

		// Add results if we found any URLs
		if len(urls) > 0 {
			results = append(results, SearchResult{
				Keyword: topic.Keyword,
				URLs:    urls,
			})
			fmt.Printf("Added results for %s\n", topic.Keyword)
		}
	}

	// Check if we found any results
	if len(results) == 0 {
		return nil, fmt.Errorf("no search results found for any keywords")
	}

	return results, nil
} 