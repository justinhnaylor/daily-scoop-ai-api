package main

import (
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Global database client
var dbClient DBClient

type DBClient interface {
	SaveArticle(article *GeneratedArticle, mediaAssets NewsMediaAssets, imageSuccess bool) (*NewsArticle, error)
	CheckSimilarKeywords(keyword string, hours int) (bool, error)
}

// Models
type NewsArticle struct {
	ID         uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	Title      string        `gorm:"not null;type:text"`
	Body       string        `gorm:"not null;type:text"`
	ImageUrl   *string       `gorm:"column:imageUrl"`
	ThumbnailUrl *string     `gorm:"column:thumbnailUrl"`
	AudioUrl   *string       `gorm:"column:audioUrl"`
	AuthorId   string        `gorm:"column:authorId;type:uuid;not null"`
	CategoryId *int          `gorm:"column:categoryId"`
	Keywords   pq.StringArray `gorm:"type:text[];default:'{}'"`
	CreatedAt  time.Time     `gorm:"column:createdAt;default:CURRENT_TIMESTAMP"`
	UpdatedAt  time.Time     `gorm:"column:updatedAt"`
	Published  bool          `gorm:"default:false"`
	URLTitle   string        `gorm:"column:urlTitle"`
	UseImage   bool          `gorm:"column:useImage;default:true"`
}

type User struct {
	ID string `gorm:"type:uuid;primary_key"`
	// ... other User fields ...
}

type Category struct {
	ID int `gorm:"primary_key"`
	// ... other Category fields ...
}

func (NewsArticle) TableName() string {
	return "news_article"
}

// SupabaseClient implementation
type SupabaseClient struct {
	db *gorm.DB
}

func NewSupabaseClient(dbURL, apiKey string) (*SupabaseClient, error) {
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Supabase database: %v", err)
	}
	return &SupabaseClient{db: db}, nil
}

func (s *SupabaseClient) SaveArticle(article *GeneratedArticle, mediaAssets NewsMediaAssets, imageSuccess bool) (*NewsArticle, error) {
	// Ensure the original keyword is included in the keywords array
	keywords := article.Keywords
	if !contains(keywords, article.Keyword) {
		keywords = append([]string{article.Keyword}, keywords...)
	}

	newsArticle := &NewsArticle{
		ID:           uuid.New(),
		Title:        article.Title,
		Body:         article.Article,
		ImageUrl:     &mediaAssets.ImagePath,
		ThumbnailUrl: &mediaAssets.ThumbnailPath,
		AudioUrl:     &mediaAssets.AudioPath,
		AuthorId:     "a66dd82e-9e8e-44e8-94fa-825dd1cd2f7c",
		CategoryId:   &article.CategoryId,
		Keywords:     pq.StringArray(keywords),
		Published:    true,
		URLTitle:     article.URLTitle,
		UseImage:     imageSuccess,
	}

	if err := s.db.Create(newsArticle).Error; err != nil {
		return nil, fmt.Errorf("error saving to Supabase database: %v", err)
	}

	return newsArticle, nil
}

// Helper function to check if a string slice contains a value
func contains(slice []string, str string) bool {
	for _, v := range slice {
		if v == str {
			return true
		}
	}
	return false
}

func (s *SupabaseClient) CheckSimilarKeywords(keyword string, hours int) (bool, error) {
	var count int64
	timeThreshold := time.Now().Add(-time.Duration(hours) * time.Hour)
	
	// Check for exact matches first
	err := s.db.Model(&NewsArticle{}).
		Where("LOWER(keywords::text) LIKE LOWER(?) AND \"createdAt\" > ?", 
			fmt.Sprintf("%%\"%s\"%%", keyword), timeThreshold).
		Count(&count).Error
	
	if err != nil {
		return false, fmt.Errorf("error checking exact keywords: %v", err)
	}
	
	if count > 0 {
		return true, nil
	}
	
	// Check for similar keywords using trigram similarity
	err = s.db.Raw(`
		SELECT COUNT(*) 
		FROM news_article, unnest(keywords) keyword 
		WHERE "createdAt" > ? 
		AND similarity(LOWER(keyword), LOWER(?)) > 0.8`,
		timeThreshold, keyword).
		Count(&count).Error
	
	if err != nil {
		return false, fmt.Errorf("error checking similar keywords: %v", err)
	}
	
	return count > 0, nil
}

// LocalDBClient implementation
type LocalDBClient struct {
	db *gorm.DB
}

func NewLocalDBClient() (*LocalDBClient, error) {
	dsn := os.Getenv("LOCAL_DB_URL")
	if dsn == "" {
		return nil, fmt.Errorf("LOCAL_DB_URL environment variable is not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}

	// Initialize database schema
	db.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\";")
	db.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm;")
	db.Exec(`ALTER TABLE news_article DROP CONSTRAINT IF EXISTS news_article_authorId_fkey;`)
	db.Exec(`
        DO $$ 
        BEGIN
            IF EXISTS (
                SELECT 1 
                FROM information_schema.columns 
                WHERE table_name = 'user' 
                AND column_name = 'id' 
                AND data_type != 'uuid'
            ) THEN
                ALTER TABLE "user" ALTER COLUMN id TYPE uuid USING id::uuid;
            END IF;

            IF EXISTS (
                SELECT 1 
                FROM information_schema.columns 
                WHERE table_name = 'news_article' 
                AND column_name = 'authorId' 
                AND data_type != 'uuid'
            ) THEN
                ALTER TABLE news_article ALTER COLUMN "authorId" TYPE uuid USING "authorId"::uuid;
            END IF;
        END $$;
    `)
	db.Exec(`
        ALTER TABLE news_article
        ADD CONSTRAINT news_article_authorId_fkey
        FOREIGN KEY ("authorId")
        REFERENCES "user" (id)
        ON DELETE CASCADE;
    `)

	return &LocalDBClient{db: db}, nil
}

func (l *LocalDBClient) SaveArticle(article *GeneratedArticle, mediaAssets NewsMediaAssets, imageSuccess bool) (*NewsArticle, error) {
	newsArticle := &NewsArticle{
		ID:           uuid.New(),
		Title:        article.Title,
		Body:         article.Article,
		ImageUrl:     &mediaAssets.ImagePath,
		ThumbnailUrl: &mediaAssets.ThumbnailPath,
		AudioUrl:     &mediaAssets.AudioPath,
		AuthorId:     "a66dd82e-9e8e-44e8-94fa-825dd1cd2f7c",
		CategoryId:   &article.CategoryId,
		Keywords:     pq.StringArray(article.Keywords),
		Published:    true,
		UseImage:     imageSuccess,
	}

	if err := l.db.Create(newsArticle).Error; err != nil {
		return nil, fmt.Errorf("error saving to local database: %v", err)
	}

	return newsArticle, nil
}

func (l *LocalDBClient) CheckSimilarKeywords(keyword string, hours int) (bool, error) {
	var count int64
	timeThreshold := time.Now().Add(-time.Duration(hours) * time.Hour)
	
	// Check for exact matches first
	err := l.db.Model(&NewsArticle{}).
		Where("LOWER(keywords::text) LIKE LOWER(?) AND \"createdAt\" > ?", 
			fmt.Sprintf("%%\"%s\"%%", keyword), timeThreshold).
		Count(&count).Error
	
	if err != nil {
		return false, fmt.Errorf("error checking exact keywords: %v", err)
	}
	
	if count > 0 {
		return true, nil
	}
	
	// Check for similar keywords using trigram similarity
	err = l.db.Raw(`
		SELECT COUNT(*) 
		FROM news_article, unnest(keywords) keyword 
		WHERE "createdAt" > ? 
		AND similarity(LOWER(keyword), LOWER(?)) > 0.8`,
		timeThreshold, keyword).
		Count(&count).Error
	
	if err != nil {
		return false, fmt.Errorf("error checking similar keywords: %v", err)
	}
	
	return count > 0, nil
}

func initDB() error {
	dbType := os.Getenv("DB_TYPE")
	
	switch dbType {
	case "prod":
		dbURL := os.Getenv("SUPABASE_URL")
		if dbURL == "" {
			return fmt.Errorf("SUPABASE_URL environment variable is not set")
		}
		apiKey := os.Getenv("SUPABASE_ANON_KEY")
		if apiKey == "" {
			return fmt.Errorf("SUPABASE_ANON_KEY environment variable is not set")
		}
		client, err := NewSupabaseClient(dbURL, apiKey)
		if err != nil {
			return fmt.Errorf("error initializing Supabase client: %v", err)
		}
		dbClient = client
		
	case "local", "":
		localClient, err := NewLocalDBClient()
		if err != nil {
			return fmt.Errorf("error initializing local database: %v", err)
		}
		dbClient = localClient
		
	default:
		return fmt.Errorf("unknown database type: %s", dbType)
	}
	
	return nil
} 