package models

import (
	"time"

	"gorm.io/gorm"
)

type PlanType string

const (
	PlanFree       PlanType = "free"
	PlanBasic      PlanType = "basic"
	PlanPro        PlanType = "pro"
	PlanEnterprise PlanType = "enterprise"
)

type Plan struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	Name            PlanType       `gorm:"uniqueIndex;not null;size:50" json:"name"`
	DisplayName     string         `gorm:"not null;size:100" json:"display_name"`
	Description     string         `gorm:"type:text" json:"description"`
	Price           float64        `gorm:"not null" json:"price"`
	Currency        string         `gorm:"default:USD;size:3" json:"currency"`
	BillingCycle    string         `gorm:"default:monthly;size:20" json:"billing_cycle"`
	CreditsPerMonth int            `gorm:"not null" json:"credits_per_month"`
	MaxGenerations  int            `gorm:"default:-1" json:"max_generations"`
	Features        string         `gorm:"type:jsonb" json:"features"`
	IsActive        bool           `gorm:"default:true" json:"is_active"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

type Subscription struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	UserID              uint           `gorm:"uniqueIndex;not null" json:"user_id"`
	PlanID              uint           `gorm:"not null" json:"plan_id"`
	Status              string         `gorm:"default:active;size:20" json:"status"`
	CurrentPeriodStart  time.Time      `json:"current_period_start"`
	CurrentPeriodEnd    time.Time      `json:"current_period_end"`
	CancelAtPeriodEnd   bool           `gorm:"default:false" json:"cancel_at_period_end"`
	PaymentProvider     string         `gorm:"size:50" json:"payment_provider,omitempty"`
	PaymentProviderID   string         `gorm:"size:100" json:"payment_provider_id,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
	User                User           `gorm:"foreignKey:UserID" json:"-"`
	Plan                Plan           `gorm:"foreignKey:PlanID" json:"plan"`
}

type CreditTransaction struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	UserID        uint           `gorm:"index;not null" json:"user_id"`
	Amount        int            `gorm:"not null" json:"amount"`
	Type          string         `gorm:"not null;size:20" json:"type"`
	Description   string         `gorm:"size:255" json:"description"`
	GenerationID  *uint          `json:"generation_id,omitempty"`
	BalanceBefore int            `json:"balance_before"`
	BalanceAfter  int            `json:"balance_after"`
	CreatedAt     time.Time      `json:"created_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

var DefaultPlans = []Plan{
	{
		Name:            PlanFree,
		DisplayName:     "Free",
		Description:     "Get started with basic features",
		Price:           0,
		Currency:        "USD",
		BillingCycle:    "monthly",
		CreditsPerMonth: 10,
		MaxGenerations:  50,
		Features:        `["10 credits/month", "Basic music generation", "720p video", "Community support"]`,
		IsActive:        true,
	},
	{
		Name:            PlanBasic,
		DisplayName:     "Basic",
		Description:     "For hobbyists and casual creators",
		Price:           9.99,
		Currency:        "USD",
		BillingCycle:    "monthly",
		CreditsPerMonth: 100,
		MaxGenerations:  500,
		Features:        `["100 credits/month", "Advanced music generation", "1080p video", "Email support", "Download in multiple formats"]`,
		IsActive:        true,
	},
	{
		Name:            PlanPro,
		DisplayName:     "Pro",
		Description:     "For professional creators",
		Price:           29.99,
		Currency:        "USD",
		BillingCycle:    "monthly",
		CreditsPerMonth: 500,
		MaxGenerations:  -1,
		Features:        `["500 credits/month", "Unlimited generations", "4K video", "Priority support", "API access", "Custom styles"]`,
		IsActive:        true,
	},
	{
		Name:            PlanEnterprise,
		DisplayName:     "Enterprise",
		Description:     "For teams and businesses",
		Price:           99.99,
		Currency:        "USD",
		BillingCycle:    "monthly",
		CreditsPerMonth: 2000,
		MaxGenerations:  -1,
		Features:        `["2000 credits/month", "Unlimited everything", "8K video", "Dedicated support", "Custom API limits", "White-label option", "SLA guarantee"]`,
		IsActive:        true,
	},
}
