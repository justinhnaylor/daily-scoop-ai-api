// webshare.go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// ProxyResponse structure to match Webshare's API response
type ProxyResponse struct {
	Results []struct {
		Username     string `json:"username"`
		Password     string `json:"password"`
		ProxyAddress string `json:"proxy_address"`
		Port         int    `json:"port"`
	} `json:"results"`
}

// WebshareAPIClient to interact with Webshare API
func WebshareAPIClient() (*http.Client, error) {
	client := &http.Client{}
	return client, nil
}

// GetProxies fetches the proxies from Webshare API
func GetProxies() ([]string, error) {
	apiKey := os.Getenv("WEBSHARE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("WEBSHARE_API_KEY environment variable not set")
	}

	url := "https://proxy.webshare.io/api/v2/proxy/list/?mode=direct&page=1&page_size=25"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	// Set headers with API key for authentication
	req.Header.Add("Authorization", fmt.Sprintf("Token %s", apiKey))

	client, err := WebshareAPIClient()
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %v", err)
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch proxies: %v", resp.Status)
	}

	// Parse response JSON
	var proxyResp ProxyResponse
	if err := json.NewDecoder(resp.Body).Decode(&proxyResp); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}

	// Extract proxy URLs
	var proxyUrls []string
	for _, p := range proxyResp.Results {
		// Format: protocol://username:password@proxy_address:port
		proxyUrl := fmt.Sprintf("http://%s:%s@%s:%d", 
			p.Username, 
			p.Password, 
			p.ProxyAddress, 
			p.Port)
		proxyUrls = append(proxyUrls, proxyUrl)
	}

	return proxyUrls, nil
}
