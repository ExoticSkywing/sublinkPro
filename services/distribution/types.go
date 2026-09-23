// Package distribution owns the fork's subscription delivery business. It does
// not reuse legacy share tokens, caches, or expiry rules.
package distribution

import (
	"time"

	"gorm.io/gorm"
)

type Settings struct {
	ID                     int       `gorm:"primaryKey" json:"-"`
	Revision               int       `json:"-"`
	TrialDays              int       `json:"trial_days"`
	FreeDays               int       `json:"free_days"`
	CycleDays              int       `json:"cycle_days"`
	RegionLimit            int       `gorm:"not null;default:2" json:"region_limit"`
	CycleAnchor            time.Time `json:"cycle_anchor"`
	PortalURL              string    `json:"portal_url"`
	Domain                 string    `json:"domain"`
	AllowedUA              string    `json:"allowed_ua"`
	FallbackSubscriptionID int       `json:"fallback_subscription_id"`
	ExpiredMessage         string    `json:"expired_message"`
	ExpiredMessages        []string  `gorm:"serializer:json;type:text" json:"expired_messages"`
}

func (Settings) TableName() string { return "distribution_settings" }

type Credential struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Name           string     `gorm:"size:100" json:"name"`
	Batch          string     `gorm:"size:100;index" json:"batch"`
	SubscriptionID int        `gorm:"index" json:"subscription_id"`
	Secret         string     `gorm:"type:text" json:"-"`
	TokenHash      string     `gorm:"size:64;uniqueIndex" json:"-"`
	Token          string     `gorm:"-" json:"token,omitempty"`
	Status         string     `gorm:"size:20;index" json:"status"`
	Plan           string     `gorm:"size:20" json:"plan"`
	TrialDays      int        `json:"trial_days"`
	ActivatedAt    *time.Time `json:"activated_at"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Permanent      bool       `json:"permanent"`
	LastAccessAt   *time.Time `json:"last_access_at"`
	AccessCount    int64      `json:"access_count"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Regions        []Region   `gorm:"-" json:"regions,omitempty"`
}

func (Credential) TableName() string { return "distribution_credentials" }
func (c Credential) Active(now time.Time) bool {
	return c.Status == "enabled" && (c.ActivatedAt == nil || c.Permanent || (c.ExpiresAt != nil && now.Before(*c.ExpiresAt)))
}

type City struct {
	Key      string `json:"key"`
	Country  string `json:"country"`
	Province string `json:"province"`
	Name     string `json:"name"`
}
type Region struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	CredentialID uint      `gorm:"uniqueIndex:distribution_region_unique" json:"credential_id"`
	CityKey      string    `gorm:"size:100;uniqueIndex:distribution_region_unique" json:"city_key"`
	Province     string    `gorm:"size:100" json:"province"`
	City         string    `gorm:"size:100" json:"city"`
	CreatedAt    time.Time `json:"created_at"`
}

func (Region) TableName() string { return "distribution_regions" }

type Card struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	Name              string         `gorm:"size:100" json:"name"`
	Batch             string         `gorm:"size:100;index" json:"batch"`
	Kind              string         `gorm:"size:20" json:"kind"`
	Secret            string         `gorm:"type:text" json:"-"`
	CodeHash          string         `gorm:"size:64;uniqueIndex" json:"-"`
	Code              string         `gorm:"-" json:"code,omitempty"`
	Enabled           bool           `json:"enabled"`
	Cycle             string         `gorm:"size:100;index" json:"cycle"`
	Days              int            `json:"days"`
	StartsAt          time.Time      `json:"starts_at"`
	EndsAt            *time.Time     `json:"ends_at"`
	BoundCredentialID *uint          `json:"bound_credential_id"`
	RedemptionCount   int64          `json:"redemption_count"`
	CreatedAt         time.Time      `json:"created_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Card) TableName() string { return "distribution_cards" }

type Redemption struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	CredentialID uint       `gorm:"uniqueIndex:distribution_redemption_unique;index" json:"credential_id"`
	BenefitKey   string     `gorm:"size:120;uniqueIndex:distribution_redemption_unique" json:"-"`
	CardID       uint       `gorm:"index" json:"card_id"`
	Kind         string     `gorm:"size:20" json:"kind"`
	Before       *time.Time `json:"before"`
	After        *time.Time `json:"after"`
	Permanent    bool       `json:"permanent"`
	Source       string     `gorm:"size:20" json:"source"`
	CreatedAt    time.Time  `json:"created_at"`
	// Response-only: an idempotent retry did not grant another benefit.
	AlreadyRedeemed bool `gorm:"-" json:"already_redeemed"`
}

func (Redemption) TableName() string { return "distribution_redemptions" }

type Visit struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	CredentialID uint      `gorm:"index" json:"credential_id"`
	IP           string    `gorm:"size:64" json:"ip"`
	UA           string    `gorm:"size:512" json:"ua"`
	Country      string    `gorm:"size:8" json:"country"`
	CityKey      string    `gorm:"size:100" json:"city_key"`
	Province     string    `gorm:"size:100" json:"province"`
	City         string    `gorm:"size:100" json:"city"`
	Result       string    `gorm:"size:40" json:"result"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

func (Visit) TableName() string { return "distribution_visits" }

type RegionRequest struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	CredentialID uint       `gorm:"index" json:"credential_id"`
	VisitID      uint       `json:"visit_id"`
	CityKey      string     `gorm:"size:100" json:"city_key"`
	Province     string     `gorm:"size:100" json:"province"`
	City         string     `gorm:"size:100" json:"city"`
	Reason       string     `gorm:"size:1000" json:"reason"`
	Contact      string     `gorm:"size:200" json:"contact"`
	Status       string     `gorm:"size:20;index" json:"status"`
	Reply        string     `gorm:"size:1000" json:"reply"`
	ReviewedBy   string     `gorm:"size:100" json:"reviewed_by"`
	CreatedAt    time.Time  `json:"created_at"`
	ReviewedAt   *time.Time `json:"reviewed_at"`
}

func (RegionRequest) TableName() string { return "distribution_region_requests" }

type Problem string

func (p Problem) Error() string { return string(p) }

const (
	Invalid             Problem = "invalid_input"
	NotFound            Problem = "not_found"
	Inactive            Problem = "inactive"
	NotActivated        Problem = "not_activated"
	CardUnavailable     Problem = "card_unavailable"
	CardUsed            Problem = "card_used"
	CardLocked          Problem = "card_locked"
	CardDeleted         Problem = "card_deleted"
	Covered             Problem = "already_covered"
	Conflict            Problem = "conflict"
	TooFrequent         Problem = "too_frequent"
	RegionLimitConflict Problem = "region_limit_conflict"
)

// AccessGrant is a short-lived, administrator-approved exception for a
// client whose request reaches the service without a usable location. It is
// deliberately scoped to one credential and one exact public IP.
type AccessGrant struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	CredentialID uint       `gorm:"index" json:"credential_id"`
	IP           string     `gorm:"size:64;index" json:"ip"`
	Reason       string     `gorm:"size:500" json:"reason"`
	ExpiresAt    time.Time  `json:"expires_at"`
	Enabled      bool       `json:"enabled"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (AccessGrant) TableName() string { return "distribution_access_grants" }

func (g AccessGrant) Active(now time.Time) bool {
	return g.Enabled && g.RevokedAt == nil && now.Before(g.ExpiresAt)
}
