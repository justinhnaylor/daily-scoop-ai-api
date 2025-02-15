package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/playwright-community/playwright-go"
)

// TrendingTopic represents a single trending topic with all its data
type TrendingTopic struct {
	Keyword         string   `json:"keyword"`
	SearchVolume    string   `json:"searchVolume"`
	Status          string   `json:"status"`
	TimeAgo         string   `json:"timeAgo"`
	TrendBreakdown  []string `json:"trendBreakdown"`
}

// Update constants at the top of the file
const (
	MAX_DAILY_TOPICS  = 5  // Or whatever number you want for daily
	MAX_RECENT_TOPICS = 5  // Or whatever number you want for recent
)

// GetTrendingKeywords fetches trending keywords from Google Trends using Playwright and Webshare proxies
func GetTrendingKeywords() ([]TrendingTopic, error) {
	// Fetch proxies from Webshare API
	proxies, err := GetProxies()
	if err != nil {
		return nil, fmt.Errorf("error fetching proxies: %v", err)
	}

	if len(proxies) == 0 {
		return nil, fmt.Errorf("no proxies found")
	}

	// Use the first proxy from the list
	proxy := proxies[0]

	// Initialize Playwright and launch browser
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("could not start Playwright: %v", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("could not launch browser: %v", err)
	}
	defer browser.Close()

	// Set up context with proxy
	contextOptions := playwright.BrowserNewContextOptions{
		Proxy: &playwright.Proxy{
			Server: proxy,
		},
	}

	context, err := browser.NewContext(contextOptions)
	if err != nil {
		return nil, fmt.Errorf("could not create browser context: %v", err)
	}
	defer context.Close()

	page, err := context.NewPage()
	if err != nil {
		return nil, fmt.Errorf("could not create page: %v", err)
	}
	defer page.Close()

	// Navigate to Google Trends
	if _, err = page.Goto("https://trends.google.com/trending?geo=US&hours=24", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		return nil, fmt.Errorf("could not go to Google Trends: %v", err)
	}

	// Wait for the content to be visible
	time.Sleep(2 * time.Second)

	// Get the page content and parse with goquery
	content, err := page.Content()
	if err != nil {
		return nil, fmt.Errorf("could not get page content: %v", err)
	}

	// Parse the HTML content
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("could not parse HTML: %v", err)
	}

	var topics []TrendingTopic

	func() {
		defer func() {
			recover() // Recover from our intentional panic
		}()

		doc.Find("table tbody:nth-of-type(2) tr").Each(func(i int, s *goquery.Selection) {
			// Skip header row if present
			cells := s.Find("td")
			if cells.Length() < 2 {
				return
			}

			// Collect all related search terms from the 5th cell (index 4)
			var relatedTerms []string
			cells.Eq(4).Find("button span:nth-child(4)").Each(func(i int, s *goquery.Selection) {
				term := strings.TrimSpace(s.Text())
				if term != "" {
					relatedTerms = append(relatedTerms, term)
				}
			})

			topic := TrendingTopic{
				Keyword:         strings.TrimSpace(cells.Eq(1).Children().First().Text()),
				SearchVolume:    strings.TrimSpace(cells.Eq(1).Find("div:nth-child(2) > div:first-child > div:first-child").Text()),
				Status:          strings.TrimSpace(cells.Eq(1).Find("div:nth-child(2) > div:nth-child(2) > div:last-child").Text()),
				TimeAgo:         strings.TrimSpace(cells.Eq(1).Find("div:nth-child(2) > div:nth-child(3) > div:last-child").Text()),
				TrendBreakdown:  relatedTerms,
			}

			// Check if topic is news-related using DeepSeek before adding
			isNewsRelated, replacementKeyword, err := IsNewsRelatedTopic(topic.Keyword, topic.TrendBreakdown)
			if err != nil {
				fmt.Printf("Warning: Could not check if '%s' is news-related: %v\n", topic.Keyword, err)
				return
			}

			// Only append active and news-related topics
			if topic.Keyword != "" && topic.Status == "Active" {
				if isNewsRelated {
					// If we have a replacement keyword, use it
					if replacementKeyword != "" {
						topic.Keyword = replacementKeyword
					}

					// Check if we already have a similar article in the database from the last 24 hours
					similar, err := dbClient.CheckSimilarKeywords(topic.Keyword, 24) // Check last 24 hours
					if err != nil {
						fmt.Printf("Warning: Error checking database for similar keywords '%s': %v\n", topic.Keyword, err)
						return
					}

					if !similar {
						topics = append(topics, topic)
						fmt.Printf("Added unique topic: %s\n", topic.Keyword)
						// Break if we've reached MAX_TRENDING_TOPICS
						if len(topics) >= MAX_DAILY_TOPICS {
							s.Parent().Find("tr").Each(func(_ int, _ *goquery.Selection) {
								panic("break")
							})
						}
					} else {
						fmt.Printf("Skipping topic '%s' - similar article exists in database\n", topic.Keyword)
					}
				}
			}
		})
	}()

	if len(topics) == 0 {
		return nil, fmt.Errorf("no trending topics found")
	}

	// Filter out topics with similar keywords in the database
	var filteredTopics []TrendingTopic
	for _, topic := range topics {
		fmt.Printf("\nChecking similarity for topic: %s\n", topic.Keyword)
		similar, err := CheckSimilarKeywords(topic.Keyword, topicsToKeywords(filteredTopics)) // Pass filteredTopics keywords for similarity check
		if err != nil {
			fmt.Printf("Warning: Error checking similar keywords for '%s': %v\n", topic.Keyword, err)
			continue
		}
		fmt.Printf("Similarity check result for '%s': similar=%v\n", topic.Keyword, similar)

		if !similar {
			filteredTopics = append(filteredTopics, topic)
			fmt.Printf("Found unique topic: %s\n", topic.Keyword)
			// If we've reached our limit, break
			if len(filteredTopics) >= MAX_DAILY_TOPICS {
				break
			}
		} else {
			fmt.Printf("Skipping similar keyword: %s\n", topic.Keyword)
		}
	}

	// If we found any unique topics, return them
	if len(filteredTopics) > 0 {
		return filteredTopics, nil
	}

	// If all topics were filtered out, return error
	fmt.Printf("No unique topics found. All were similar to recent articles.\n")
	return nil, fmt.Errorf("all trending topics were similar to recent articles")
}

// Helper function to extract keywords from TrendingTopic slice
func topicsToKeywords(topics []TrendingTopic) []string {
	keywords := make([]string, len(topics))
	for i, topic := range topics {
		keywords[i] = topic.Keyword
	}
	return keywords
}


func extractTrendingKeywords(doc *goquery.Document) []string {
	var keywords []string

	// Find all table rows in the trending topics table
	doc.Find("tr").Each(func(i int, s *goquery.Selection) {
		// Look for the keyword text in the first content cell of each row
		keyword := strings.TrimSpace(s.Find("td:nth-child(2)").Text())
		if keyword != "" {
			keywords = append(keywords, keyword)
		}
	})

	return keywords
}

// FormatTrendingTopicsJSON formats the trending topics as a JSON object with metadata
func FormatTrendingTopicsJSON(topics []TrendingTopic) (string, error) {
	type TrendingData struct {
		Timestamp string         `json:"timestamp"`
		Count     int           `json:"count"`
		Topics    []TrendingTopic `json:"topics"`
	}

	data := TrendingData{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Count:     len(topics),
		Topics:    topics,
	}

	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling to JSON: %v", err)
	}

	return string(jsonBytes), nil
}

// IsNewsRelatedTopic uses Gemini to determine if a keyword is news-related
func IsNewsRelatedTopic(keyword string, trendBreakdown []string) (bool, string, error) {
	prompt := fmt.Sprintf(`Analyze if the trending topic '%s' is specifically related to CURRENT news or breaking events happening right now.

Rules:
- Answer in this format: "<true/false>|<replacement_keyword>"
- If the topic is specific enough and news-related, respond with "true|"
- If the topic is a **single vague word** but news-related AND there are related terms, pick the most newsworthy related term and respond with "true|<selected_term>"
- If the topic is a **phrase or already specific**, do not replace it; respond with "true|"
- If the topic is too vague and there are no related terms, respond with "false|"
- If the topic is not news-related, respond with "false|"

Related terms for this topic: %v

For example:
- Specific news topic: "Ukraine conflict" -> "true|"
- Single vague word with related terms: "UFC" with related term "UFC 300 main event cancelled" -> "true|UFC 300 main event cancelled"
- Vague phrase that shouldn't be replaced: "market trends" -> "true|"
- Non-news or too vague without related terms: "recipes" -> "false|"

Analyze '%s' and provide your response:`, keyword, trendBreakdown, keyword)

	response, err := QueryGemini(prompt) // Changed to QueryGemini
	if err != nil {
		return false, "", err
	}

	fmt.Printf("Gemini Response for '%s': %s\n", keyword, response) // Debug print

	parts := strings.Split(response, "|")
	if len(parts) != 2 {
		return false, "", fmt.Errorf("invalid response format: %s", response)
	}

	isNews := strings.TrimSpace(strings.ToLower(parts[0])) == "true"
	replacementKeyword := strings.TrimSpace(parts[1])

	fmt.Printf("Parsed Response - IsNews: %v, Replacement: '%s'\n", isNews, replacementKeyword) // Debug print

	// If it's news-related and we have a replacement keyword
	if isNews && replacementKeyword != "" {
		fmt.Printf("Using replacement keyword: '%s'\n", replacementKeyword) // Debug print
		return true, replacementKeyword, nil
	}

	return isNews, "", nil
}

// QueryGemini sends a prompt to Gemini API and returns the response // Renamed to QueryGemini
func QueryGemini(prompt string) (string, error) {
	client := &http.Client{}

	// Prepare the request body for Gemini API
	requestBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{
						"text": prompt,
					},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %v", err)
	}

	// Create the request to Gemini API endpoint
	apiKey := os.Getenv("GEMINI_API_KEY") // Use GEMINI_API_KEY
	apiEndpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=%s", apiKey) // Using gemini-pro

	req, err := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	// Set headers for Gemini API
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request to Gemini API: %v", err)
	}
	defer resp.Body.Close()

	// Check for non-OK status codes
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini API request failed with status code: %d", resp.StatusCode)
	}

	// Read and parse the Gemini API response
	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("error decoding Gemini API response: %v", err)
	}

	candidates, ok := response["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return "", fmt.Errorf("no candidates found in Gemini API response")
	}
	candidate := candidates[0].(map[string]interface{})
	content, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("content not found in candidate")
	}
	parts, ok := content["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return "", fmt.Errorf("parts not found in content")
	}
	part := parts[0].(map[string]interface{})
	geminiResponse, ok := part["text"].(string)
	if !ok {
		return "", fmt.Errorf("text not found in part")
	}

	geminiResponse = strings.TrimSpace(geminiResponse)
	fmt.Printf("Raw Gemini Response: '%s'\n", geminiResponse) // Added raw response print

	// Validate response format (should be "true|replacement" or "false|" for IsNewsRelated, and "true" or "false" for CheckSimilarKeywords)
	responseParts := strings.Split(geminiResponse, "|")
	if len(responseParts) == 2 {
		isNews := strings.ToLower(strings.TrimSpace(responseParts[0]))
		if isNews != "true" && isNews != "false" {
			fmt.Printf("Warning: Invalid boolean value in response from Gemini (2 parts): %s\n", geminiResponse)
			return "false|", nil // Default to false if invalid format
		}
		return geminiResponse, nil // Return full response for IsNewsRelated
	} else if len(responseParts) == 1 {
		isBool := strings.ToLower(strings.TrimSpace(responseParts[0]))
		if isBool == "true" || isBool == "false" {
			return geminiResponse, nil // Return single "true" or "false" for CheckSimilarKeywords
		} else {
			fmt.Printf("Warning: Invalid boolean value in response from Gemini (1 part): %s\n", geminiResponse)
			return "false", fmt.Errorf("invalid boolean response from Gemini: %s", geminiResponse) // Indicate error for unexpected single part response
		}
	} else {
		fmt.Printf("Warning: Received unexpected response format from Gemini: %s\n", geminiResponse)
		return "false", fmt.Errorf("unexpected response format from Gemini: %s", geminiResponse) // Indicate error for completely unexpected format
	}
}


// CheckSimilarKeywords compares a new keyword with existing keywords and returns true if they are similar
func CheckSimilarKeywords(newKeyword string, existingKeywords []string) (bool, error) {
	if len(existingKeywords) == 0 {
		return false, nil
	}

	// Simplified prompt for more reliable responses
	prompt := fmt.Sprintf(`Compare if this keyword "%s" is semantically similar to any of these keywords: %v.

Answer with ONLY a single word: "true" or "false"`, newKeyword, existingKeywords)

	response, err := QueryGemini(prompt)
	if err != nil {
		return false, fmt.Errorf("error querying Gemini for similarity: %v", err)
	}

	// Clean and validate the response
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "true", nil
}

func GetTrendingKeywordsWithMode(mode string) ([]TrendingTopic, error) {
	var (
		url string
		maxTopics int
	)
	
	switch mode {
	case "daily":
		url = "https://trends.google.com/trending?geo=US&hours=24"
		maxTopics = MAX_DAILY_TOPICS
	case "recent":
		url = "https://trends.google.com/trending?geo=US&hours=2"
		maxTopics = MAX_RECENT_TOPICS
	default:
		return nil, fmt.Errorf("invalid mode: %s", mode)
	}

	// Pass both URL and max topics limit
	return GetTrendingKeywordsFromURL(url, maxTopics)
}

func GetTrendingKeywordsFromURL(trendURL string, maxTopics int) ([]TrendingTopic, error) {
	// Fetch proxies from Webshare API
	proxies, err := GetProxies()
	if err != nil {
		return nil, fmt.Errorf("error fetching proxies: %v", err)
	}

	if len(proxies) == 0 {
		return nil, fmt.Errorf("no proxies found")
	}

	// Use the first proxy from the list
	proxy := proxies[0]

	// Initialize Playwright and launch browser
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("could not start Playwright: %v", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("could not launch browser: %v", err)
	}
	defer browser.Close()

	// Set up context with proxy
	contextOptions := playwright.BrowserNewContextOptions{
		Proxy: &playwright.Proxy{
			Server: proxy,
		},
	}

	context, err := browser.NewContext(contextOptions)
	if err != nil {
		return nil, fmt.Errorf("could not create browser context: %v", err)
	}
	defer context.Close()

	page, err := context.NewPage()
	if err != nil {
		return nil, fmt.Errorf("could not create page: %v", err)
	}
	defer page.Close()

	// Navigate to the provided URL
	if _, err = page.Goto(trendURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		return nil, fmt.Errorf("could not go to Google Trends: %v", err)
	}

	// Wait for the content to be visible
	time.Sleep(2 * time.Second)

	// Get the page content and parse with goquery
	content, err := page.Content()
	if err != nil {
		return nil, fmt.Errorf("could not get page content: %v", err)
	}

	// Parse the HTML content
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("could not parse HTML: %v", err)
	}

	var topics []TrendingTopic

	func() {
		defer func() {
			recover() // Recover from our intentional panic
		}()

		doc.Find("table tbody:nth-of-type(2) tr").Each(func(i int, s *goquery.Selection) {
			// Skip header row if present
			cells := s.Find("td")
			if cells.Length() < 2 {
				return
			}

			// Collect all related search terms
			var relatedTerms []string
			cells.Eq(4).Find("button span:nth-child(4)").Each(func(i int, s *goquery.Selection) {
				term := strings.TrimSpace(s.Text())
				if term != "" {
					relatedTerms = append(relatedTerms, term)
				}
			})

			topic := TrendingTopic{
				Keyword:        strings.TrimSpace(cells.Eq(1).Children().First().Text()),
				SearchVolume:   strings.TrimSpace(cells.Eq(1).Find("div:nth-child(2) > div:first-child > div:first-child").Text()),
				Status:         strings.TrimSpace(cells.Eq(1).Find("div:nth-child(2) > div:nth-child(2) > div:last-child").Text()),
				TimeAgo:        strings.TrimSpace(cells.Eq(1).Find("div:nth-child(2) > div:nth-child(3) > div:last-child").Text()),
				TrendBreakdown: relatedTerms,
			}

			// Check if topic is news-related
			isNewsRelated, replacementKeyword, err := IsNewsRelatedTopic(topic.Keyword, topic.TrendBreakdown)
			if err != nil {
				fmt.Printf("Warning: Could not check if '%s' is news-related: %v\n", topic.Keyword, err)
				return
			}

			// Only append active and news-related topics
			if topic.Keyword != "" && topic.Status == "Active" {
				if isNewsRelated {
					// Use replacement keyword if available
					if replacementKeyword != "" {
						topic.Keyword = replacementKeyword
					}

					// Check for similar articles in database
					similar, err := dbClient.CheckSimilarKeywords(topic.Keyword, 24)
					if err != nil {
						fmt.Printf("Warning: Error checking database for similar keywords '%s': %v\n", topic.Keyword, err)
						return
					}

					if !similar {
						topics = append(topics, topic)
						fmt.Printf("Added unique topic: %s\n", topic.Keyword)
						if len(topics) >= maxTopics {
							panic("break") // Use panic to break out of the loop
						}
					} else {
						fmt.Printf("Skipping topic '%s' - similar article exists in database\n", topic.Keyword)
					}
				}
			}
		})
	}()

	if len(topics) == 0 {
		return nil, fmt.Errorf("no trending topics found")
	}

	// Filter out topics with similar keywords in the database
	var filteredTopics []TrendingTopic
	for _, topic := range topics {
		fmt.Printf("\nChecking similarity for topic: %s\n", topic.Keyword)
		similar, err := CheckSimilarKeywords(topic.Keyword, topicsToKeywords(filteredTopics)) // Pass filteredTopics keywords for similarity check
		if err != nil {
			fmt.Printf("Warning: Error checking similar keywords for '%s': %v\n", topic.Keyword, err)
			continue
		}
		fmt.Printf("Similarity check result for '%s': similar=%v\n", topic.Keyword, similar)

		if !similar {
			filteredTopics = append(filteredTopics, topic)
			fmt.Printf("Found unique topic: %s\n", topic.Keyword)
			// If we've reached our limit, break
			if len(filteredTopics) >= maxTopics {
				break
			}
		} else {
			fmt.Printf("Skipping similar keyword: %s\n", topic.Keyword)
		}
	}

	// If we found any unique topics, return them
	if len(filteredTopics) > 0 {
		return filteredTopics, nil
	}

	// If all topics were filtered out, return error
	fmt.Printf("No unique topics found. All were similar to recent articles.\n")
	return nil, fmt.Errorf("all trending topics were similar to recent articles")
}