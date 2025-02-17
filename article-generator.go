package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type ArticleRequest struct {
	Keyword     string            `json:"keyword"`
	Summaries   map[string]string `json:"summaries"`
	URLs        []string          `json:"urls"`
}

func GenerateArticleFromSummaries(keyword string, summaries map[string]string, urls []string) (*GeneratedArticle, error) {
	// First, filter summaries for relevance using Gemini
	relevantSummaries, err := filterRelevantSummaries(keyword, summaries)
	if err != nil {
		return nil, fmt.Errorf("error filtering summaries: %v", err)
	}

	// Only proceed if we have at least one relevant summary
	if len(relevantSummaries) == 0 {
		return nil, fmt.Errorf("no relevant summaries found for keyword: %s", keyword)
	}

	// Use existing prompt but with filtered summaries
	prompt := fmt.Sprintf(`As an **objective and data-driven news journalist**, craft a **concise, high-impact** article based on these news summaries about "%s."
Focus on a **single significant angle**—not a summary, but a **clear and factual narrative**.

Summaries of source articles:
%s`, keyword, formatSummariesForPrompt(relevantSummaries))

	prompt += `

**Guidelines for Neutral and Factual Reporting:**
1. **Identify the Core Factual Finding:**
   - Pinpoint the most **significant and verifiable fact** revealed across the summaries.
   - Discard opinions, speculation, or redundant information.

2. **Write an Informative Headline:**
   - **Max 10 words**, fact-driven, and neutral in tone. Avoid sensationalism or loaded language.

3. **Craft a Clear, Evidence-Based Story:**
   - **No paraphrasing**—synthesize insights into a **factual and objective narrative**.
   - Use **active voice, precise language, and balanced pacing**.
   - Apply markdown **sparingly and functionally** for clarity only:
     - [bold]text[/bold] for key data points or verifiable statistics
     - [italic]text[/italic] for direct quotes from sources (if present and relevant)
     - [bold-italic]text[/bold-italic] for critical, undisputed facts
     - [underline-italic]text[/underline-italic] for technical terms (used neutrally)
     - [p] for paragraph breaks (single tag, no closing tag needed)
   - **Avoid markdown for emotional emphasis or subjective interpretation.** Use it only to highlight objective information.

4. **Maintain Factual Accuracy and Neutrality:**
   - **4 paragraphs max.**
   - **First:** A clear, fact-based lead presenting the core finding.
   - **Second:**  Objective analysis, factual context, or data supporting the lead.
   - **Third:** Additional verifiable details or background information.
   - **Fourth:** A neutral concluding paragraph summarizing the factual implications.

5. **Uphold Journalistic Objectivity:**
   - **Strictly factual, avoids any form of bias, opinion, or subjective interpretation.**
   - Present information neutrally, without favoring any particular viewpoint or agenda.
   - **Actively avoid biased language, framing, and selection of facts.**

6. **Categorize the Article Objectively:**
   - Select the most **fitting category ID based on the factual topic**, not subjective interpretation or slant.
   - Categories: 1: Breaking News, 2: Politics, 3: World News, 4: Business & Finance,
     5: Technology, 6: Entertainment, 7: Sports, 8: Health & Wellness,
     9: Science, 10: Art & Culture, 11: Travel, 12: Food & Drink,
     13: Environment, 14: Lifestyle, 15: Opinion, 16: Education,
     17: Religion, 18: Other. (Note: "Opinion" should be avoided unless the summaries are explicitly about opinions, and even then, report *on* the opinion neutrally, not *express* an opinion).

7. **Format as Valid JSON:**
   - Ensure markdown is used minimally and for factual clarity only.

8. **Generate a Neutral URL Title:**
   - Create a urlTitle based on the article's factual topic.
   - Use lowercase letters, hyphens, and avoid any sensational or biased wording.

**Response Format:**
{
    "title": "Fact-Based, Informative Headline",
    "article": "Data indicates a [bold]significant shift[/bold] in key metrics. According to analyzed reports, *'The trend is undeniably towards [bold]X[/bold],'* experts confirm.[p]This development has [bold]measurable impacts[/bold] on sector Y.",
    "keywords": ["objective keyword 1", "factual keyword 2", "neutral keyword 3"],
    "categoryId": 9
	 "urlTitle": "fact-based-informative-headline"
}`

	// Query Gemini API
	response, err := queryGeminiForArticle(prompt)
	if err != nil {
		return nil, fmt.Errorf("error generating article: %v", err)
	}

	// Parse the response
	var result struct {
		Title      string   `json:"title"`
		Article    string   `json:"article"`
		Keywords   []string `json:"keywords"`
		CategoryId int      `json:"categoryId"`
		URLTitle   string   `json:"urlTitle"`
	}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("error parsing Gemini response: %v, response string: %s", err, response) // Added response string to error
	}

	// Create and return the GeneratedArticle
	article := &GeneratedArticle{
		Title:      result.Title,
		Article:    result.Article,
		Keyword:    keyword,
		Keywords:   append([]string{keyword}, result.Keywords...),
		CategoryId: result.CategoryId,
		URLTitle:   result.URLTitle,
	}

	// Validate category ID and default to "Other" if invalid
	if article.CategoryId < 1 || article.CategoryId > 18 {
		article.CategoryId = 18 // Default to "Other"
	}

	return article, nil
}

func formatSummariesForPrompt(summaries map[string]string) string {
	var builder strings.Builder
	for url, summary := range summaries {
		builder.WriteString(fmt.Sprintf("\nSource: %s\nSummary: %s\n", url, summary))
	}
	return builder.String()
}

func queryGeminiForArticle(prompt string) (string, error) {
	// Create a new client with your API key
	client, err := genai.NewClient(context.Background(), option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
	if err != nil {
		return "", fmt.Errorf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Using gemini-pro with specific configuration for JSON output
	model := client.GenerativeModel("gemini-2.0-flash") // Using Flash model for speed and cost-effectiveness
	model.SetTemperature(0.7)
	model.SetTopK(40)
	model.SetTopP(0.8)
	model.ResponseMIMEType = "application/json"

	// Generate content
	resp, err := model.GenerateContent(context.Background(), genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("Failed to generate content: %v", err)
	}

	// Check for errors in the response
	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("no candidates returned in response, possible error or safety filter: %+v", resp)
	}
	if len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content parts in the first candidate, possible empty response or error: %+v", resp)
	}

	// Extract text response
	textPart, ok := resp.Candidates[0].Content.Parts[0].(genai.Text)
	if !ok {
		return "", fmt.Errorf("expected text part in response, got: %+v", resp.Candidates[0].Content.Parts[0])
	}
	responseText := string(textPart)

	// Clean the response
	cleaned := strings.TrimSpace(responseText)
	if strings.HasPrefix(cleaned, "```json") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}

	// **Simplified Cleaning - Removed brace trimming/re-adding**
	// Additional JSON cleaning steps - REMOVED potentially problematic steps

	// Validate JSON structure
	var jsonCheck map[string]interface{}
	err = json.Unmarshal([]byte(cleaned), &jsonCheck)
	if err != nil {
		// Log the raw response for debugging
		fmt.Printf("Raw Gemini Response (Pre-Cleaning):\n%s\n", responseText)
		// If still invalid, try to extract just the JSON portion - Keep the fallback, it's useful
		jsonStart := strings.Index(responseText, "{")
		jsonEnd := strings.LastIndex(responseText, "}")
		if jsonStart >= 0 && jsonEnd > jsonStart {
			cleaned = responseText[jsonStart : jsonEnd+1]
			err = json.Unmarshal([]byte(cleaned), &jsonCheck)
			if err != nil {
				return "", fmt.Errorf("invalid JSON response after all cleaning attempts: %v, response: %s, raw_response: %s", err, cleaned, responseText) // Include raw response in error
			}
		} else {
			return "", fmt.Errorf("invalid JSON response and couldn't find valid JSON object: %v, response: %s, raw_response: %s", err, cleaned, responseText) // Include raw response in error
		}
	}

	return cleaned, nil
}

func printResponse(resp *genai.GenerateContentResponse) { // Changed to correct response type
	if resp == nil {
		fmt.Println("No response to print.")
		return
	}
	for _, candidate := range resp.Candidates {
		if candidate == nil {
			fmt.Println("Nil candidate in response.")
			continue
		}
		for _, part := range candidate.Content.Parts {
			if text, ok := part.(genai.Text); ok {
				fmt.Println("Generated Text:", string(text))
			} else {
				fmt.Printf("Non-text part received: %+v\n", part) // Handle non-text parts if expected
			}
		}
	}
}

func filterRelevantSummaries(keyword string, summaries map[string]string) (map[string]string, error) {
	relevantSummaries := make(map[string]string)

	for url, summary := range summaries {
		prompt := fmt.Sprintf(`Evaluate if this summary has ANY relevance or connection to "%s".
Consider broadly:
- Direct relevance: Is it about the same topic/event?
- Indirect relevance: Does it provide useful background/context?
- Related aspects: Does it discuss related trends/impacts/implications?
- Supporting information: Does it offer valuable supplementary details?

Be inclusive - if there's ANY reasonable connection, consider it relevant.

Summary: %s

Reply with ONLY "true" or "false" in JSON format: {"relevant": true} or {"relevant": false}`, keyword, summary) // Asking for JSON response

		responseStr, err := queryGeminiForArticle(prompt)
		if err != nil {
			return nil, fmt.Errorf("error checking summary relevance: %v", err)
		}

		var relevanceResult struct {
			Relevant bool `json:"relevant"`
		}
		err = json.Unmarshal([]byte(responseStr), &relevanceResult)
		if err != nil {
			fmt.Printf("Warning: Failed to parse relevance JSON response: %v, response string: %s. Treating as not relevant.\n", err, responseStr)
			continue // Treat as not relevant if parsing fails, and continue to next summary
		}

		if relevanceResult.Relevant {
			relevantSummaries[url] = summary
		}
	}

	return relevantSummaries, nil
}