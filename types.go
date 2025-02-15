package main

// GeneratedArticle represents a generated news article with metadata
type GeneratedArticle struct {
    Title      string
    Article    string
    Keyword    string
    Keywords   []string
    CategoryId int
    URLTitle   string
}

// NewsMediaAssets holds paths to generated media files for a news article
type NewsMediaAssets struct {
    AudioPath string
    ImagePath string
	ThumbnailPath  string  
} 