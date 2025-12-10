package models

import (
	"time"

	"gorm.io/gorm"
)

type GenerationType string
type GenerationStatus string

const (
	TypeMusic GenerationType = "music"
	TypeVideo GenerationType = "video"

	StatusPending    GenerationStatus = "pending"
	StatusProcessing GenerationStatus = "processing"
	StatusCompleted  GenerationStatus = "completed"
	StatusFailed     GenerationStatus = "failed"
)

type Generation struct {
	ID           uint             `gorm:"primaryKey" json:"id"`
	UserID       uint             `gorm:"index;not null" json:"user_id"`
	Type         GenerationType   `gorm:"not null;size:20" json:"type"`
	Status       GenerationStatus `gorm:"default:pending;size:20" json:"status"`
	Title        string           `gorm:"size:255" json:"title"`
	Prompt       string           `gorm:"type:text;not null" json:"prompt"`
	Lyrics       string           `gorm:"type:text" json:"lyrics,omitempty"`
	Narration    string           `gorm:"type:text" json:"narration,omitempty"`
	VoiceID      string           `gorm:"size:100" json:"voice_id,omitempty"`
	Style        string           `gorm:"size:100" json:"style,omitempty"`
	Duration     int              `json:"duration,omitempty"`
	Resolution   string           `gorm:"size:20" json:"resolution,omitempty"`
	Model        string           `gorm:"size:50" json:"model,omitempty"`
	OutputURL    string           `gorm:"size:500" json:"output_url,omitempty"`
	ThumbnailURL string           `gorm:"size:500" json:"thumbnail_url,omitempty"`
	MiniMaxJobID string           `gorm:"size:100" json:"minimax_job_id,omitempty"`
	ErrorMessage string           `gorm:"type:text" json:"error_message,omitempty"`
	Metadata     string           `gorm:"type:text" json:"metadata,omitempty"`
	CreditsCost  int              `gorm:"default:1" json:"credits_cost"`
	IsFavorite   bool             `gorm:"default:false" json:"is_favorite"`
	IsPublic     bool             `gorm:"default:false" json:"is_public"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	DeletedAt    gorm.DeletedAt   `gorm:"index" json:"-"`
	User         User             `gorm:"foreignKey:UserID" json:"-"`
}

type GenerationResponse struct {
	ID           uint             `json:"id"`
	UserID       uint             `json:"user_id"`
	Type         GenerationType   `json:"type"`
	Status       GenerationStatus `json:"status"`
	Title        string           `json:"title"`
	Prompt       string           `json:"prompt"`
	Lyrics       string           `json:"lyrics,omitempty"`
	Narration    string           `json:"narration,omitempty"`
	VoiceID      string           `json:"voice_id,omitempty"`
	Style        string           `json:"style,omitempty"`
	Duration     int              `json:"duration,omitempty"`
	Resolution   string           `json:"resolution,omitempty"`
	Model        string           `json:"model,omitempty"`
	OutputURL    string           `json:"output_url,omitempty"`
	ThumbnailURL string           `json:"thumbnail_url,omitempty"`
	MiniMaxJobID string           `json:"minimax_job_id,omitempty"`
	ErrorMessage string           `json:"error_message,omitempty"`
	CreditsCost  int              `json:"credits_cost"`
	IsFavorite   bool             `json:"is_favorite"`
	IsPublic     bool             `json:"is_public"`
	CreatedAt    time.Time        `json:"created_at"`
}

func (g *Generation) ToResponse() GenerationResponse {
	return GenerationResponse{
		ID:           g.ID,
		UserID:       g.UserID,
		Type:         g.Type,
		Status:       g.Status,
		Title:        g.Title,
		Prompt:       g.Prompt,
		Lyrics:       g.Lyrics,
		Narration:    g.Narration,
		VoiceID:      g.VoiceID,
		Style:        g.Style,
		Duration:     g.Duration,
		Resolution:   g.Resolution,
		Model:        g.Model,
		OutputURL:    g.OutputURL,
		ThumbnailURL: g.ThumbnailURL,
		MiniMaxJobID: g.MiniMaxJobID,
		ErrorMessage: g.ErrorMessage,
		CreditsCost:  g.CreditsCost,
		IsFavorite:   g.IsFavorite,
		IsPublic:     g.IsPublic,
		CreatedAt:    g.CreatedAt,
	}
}

type GenerateMusicRequest struct {
	Model   string `json:"model"`
	Format  string `json:"format"`
	Bitrate int    `json:"bitrate"`
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
	Lyrics string `json:"lyrics"`
	Style  string `json:"style"`
}

type GenerateVideoRequest struct {
	Title      string `json:"title"`
	Prompt     string `json:"prompt"`
	Duration   int    `json:"duration"`
	Resolution string `json:"resolution"`
	Model      string `json:"model"`
	Narration  string `json:"narration"`
	VoiceID    string `json:"voice_id"`
}

type ListGenerationsRequest struct {
	Type   string `query:"type"`
	Status string `query:"status"`
	Page   int    `query:"page"`
	Limit  int    `query:"limit"`
}
