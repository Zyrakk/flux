package models

import (
	"encoding/json"
	"time"
)

// Section represents a briefing section (e.g., Cybersecurity, Tech, Economy, World).
type Section struct {
	ID                  string          `json:"id" db:"id"`
	Name                string          `json:"name" db:"name"`
	DisplayName         string          `json:"display_name" db:"display_name"`
	Enabled             bool            `json:"enabled" db:"enabled"`
	SortOrder           int             `json:"sort_order" db:"sort_order"`
	MaxBriefingArticles int             `json:"max_briefing_articles" db:"max_briefing_articles"`
	SeedKeywords        []string        `json:"seed_keywords" db:"seed_keywords"`
	Config              json.RawMessage `json:"config,omitempty" db:"config"`
}

// Article represents an ingested article from any source.
type Article struct {
	ID             string          `json:"id" db:"id"`
	SourceType     string          `json:"source_type" db:"source_type"` // rss, hn, reddit, github, nvd
	SourceID       string          `json:"source_id" db:"source_id"`
	SectionID      *string         `json:"section_id,omitempty" db:"section_id"`
	URL            string          `json:"url" db:"url"`
	Title          string          `json:"title" db:"title"`
	Content        *string         `json:"content,omitempty" db:"content"`
	Summary        *string         `json:"summary,omitempty" db:"summary"`
	Author         *string         `json:"author,omitempty" db:"author"`
	PublishedAt    *time.Time      `json:"published_at,omitempty" db:"published_at"`
	IngestedAt     time.Time       `json:"ingested_at" db:"ingested_at"`
	ProcessedAt    *time.Time      `json:"processed_at,omitempty" db:"processed_at"`
	Embedding      []float32       `json:"embedding,omitempty" db:"embedding"`
	RelevanceScore *float64        `json:"relevance_score,omitempty" db:"relevance_score"`
	Categories     []string        `json:"categories,omitempty" db:"categories"`
	Status         string          `json:"status" db:"status"` // pending, processed, briefed, archived
	Metadata       json.RawMessage `json:"metadata,omitempty" db:"metadata"`
}

// ArticleStatus constants.
const (
	StatusPending   = "pending"
	StatusProcessed = "processed"
	StatusBriefed   = "briefed"
	StatusArchived  = "archived"
)

// Briefing represents a generated daily briefing.
type Briefing struct {
	ID          string          `json:"id" db:"id"`
	GeneratedAt time.Time       `json:"generated_at" db:"generated_at"`
	Content     string          `json:"content" db:"content"`
	ArticleIDs  []string        `json:"article_ids" db:"article_ids"`
	Metadata    json.RawMessage `json:"metadata,omitempty" db:"metadata"`
}

// Feedback represents user feedback on an article.
type Feedback struct {
	ID        string    `json:"id" db:"id"`
	ArticleID string    `json:"article_id" db:"article_id"`
	Action    string    `json:"action" db:"action"` // like, dislike, save, follow_topic
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// FeedbackAction constants.
const (
	ActionLike        = "like"
	ActionDislike     = "dislike"
	ActionSave        = "save"
	ActionFollowTopic = "follow_topic"
)

// SectionProfile holds the per-section relevance profile built from user feedback.
type SectionProfile struct {
	SectionID         string    `json:"section_id" db:"section_id"`
	PositiveEmbedding []float32 `json:"positive_embedding,omitempty" db:"positive_embedding"`
	NegativeEmbedding []float32 `json:"negative_embedding,omitempty" db:"negative_embedding"`
	LikeCount         int       `json:"like_count" db:"like_count"`
	DislikeCount      int       `json:"dislike_count" db:"dislike_count"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

// Source represents a configured content source (RSS feed, subreddit, etc.).
type Source struct {
	ID            string          `json:"id" db:"id"`
	SourceType    string          `json:"source_type" db:"source_type"` // rss, reddit, github, hn
	Name          string          `json:"name" db:"name"`
	Config        json.RawMessage `json:"config" db:"config"`
	Enabled       bool            `json:"enabled" db:"enabled"`
	LastFetchedAt *time.Time      `json:"last_fetched_at,omitempty" db:"last_fetched_at"`
	ErrorCount    int             `json:"error_count" db:"error_count"`
	LastError     *string         `json:"last_error,omitempty" db:"last_error"`
}

// SourceSection maps a source to one or more sections (many-to-many).
type SourceSection struct {
	SourceID  string `json:"source_id" db:"source_id"`
	SectionID string `json:"section_id" db:"section_id"`
}

// --- Query / Filter types ---

// ArticleFilter holds optional filters for listing articles.
type ArticleFilter struct {
	SectionID  *string
	SourceType *string
	Status     *string
	Since      *time.Time
	Until      *time.Time
	Limit      int
	Offset     int
}

// SourceFilter holds optional filters for listing sources.
type SourceFilter struct {
	SectionID  *string
	SourceType *string
	Enabled    *bool
}
