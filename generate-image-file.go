package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GetNewsImage generates an image for a news article using Gemini Flash 2
func GetNewsImage(article GeneratedArticle) (string, error) {
	// Create output directory if it doesn't exist
	outputDir := "media/images"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate unique filename using timestamp
	filename := fmt.Sprintf("image_%d.jpg", time.Now().Unix())
	outputPath := filepath.Join(outputDir, filename)

	// Get optimized prompt
	promptInstruction := fmt.Sprintf(`
Generate a photorealistic image prompt for a prestigious news publication website, based on the following news article snippet. The prompt should adhere to these guidelines to ensure high quality and avoid policy violations:

1. **Prioritize Photorealism:** The image must be photorealistic. Start with "A photo of...". Emphasize photography style.
2. **Subject is Key & News-Driven:** The image subject must be relevant to the news article's theme.
3. **Context Enhances Relevance:** Include context related to the news event (location, event, atmosphere, mood, style).
4. **Style: Journalistic Photography:** Use a journalistic photography style with natural lighting, clear focus, and authentic details.
5. **Image Quality Modifiers:** Include quality modifiers for professional, high-quality images (4K, HDR, professional photography).

Article Title: %s
First Sentence: %s

Generate ONLY the image prompt. Do not include any extra text or explanation.`, 
	article.Title, 
	strings.SplitN(article.Article, ".", 2)[0])

	// Generate the prompt using Gemini
	generatedPrompt, err := queryGeminiForPrompt(promptInstruction, "gemini-2.0-flash")
	if err != nil {
		return "", fmt.Errorf("failed to generate image prompt: %w", err)
	}

	// Call the Python script with the prompt
	cmd := exec.Command("python3", "imagen_generator.py", generatedPrompt, outputPath)
	cmd.Env = append(os.Environ(), fmt.Sprintf("GEMINI_API_KEY=%s", os.Getenv("GEMINI_API_KEY")))

	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate image: %w, output: %s", err, output)
	}

	// Verify the image was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("image file was not created")
	}

	return outputPath, nil
}

// queryGeminiForPrompt queries the Gemini API for an optimized prompt
func queryGeminiForPrompt(prompt string, modelName string) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY environment variable not set")
	}

	apiEndpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", modelName, apiKey)
	fmt.Println("Calling Gemini API Endpoint:", apiEndpoint)

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
		return "", fmt.Errorf("failed to marshal request body to JSON: %w", err)
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request to Gemini API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini API request failed with status code: %d", resp.StatusCode)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode Gemini API response: %w", err)
	}

	// Extract the generated text - adjust path based on actual Gemini API response structure
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
	generatedPrompt, ok := part["text"].(string) // Renamed to generatedPrompt
	if !ok {
		return "", fmt.Errorf("text not found in part")
	}

	fmt.Println("Gemini API Response (Generated Prompt):\n", generatedPrompt) // Now logging generatedPrompt
	return generatedPrompt, nil // Returning the Gemini-generated prompt
}
