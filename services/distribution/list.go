package distribution

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

type CardDetail struct {
	Card
	Deleted bool `json:"deleted"`
}

var searchableCardCode = regexp.MustCompile(`^(?:[23456789abcdefghjklmnpqrstuvwxyz]{16}|[0-9a-f]{64})$`)

// Authenticated audit lookup includes deleted cards, but never returns their code.
func (s *Store) CardByID(ctx context.Context, id uint) (CardDetail, error) {
	var out CardDetail
	if id == 0 {
		return out, Invalid
	}
	if err := s.DB.WithContext(ctx).Unscoped().First(&out.Card, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return out, NotFound
		}
		return out, err
	}
	out.Deleted = out.DeletedAt.Valid
	if !out.Deleted {
		var err error
		out.Code, err = s.open(out.Secret)
		return out, err
	}
	return out, nil
}

func (s *Store) ByID(ctx context.Context, id uint) (Credential, error) {
	var c Credential
	if err := s.DB.WithContext(ctx).First(&c, id).Error; err != nil {
		return c, NotFound
	}
	var err error
	if c.Status != "revoked" {
		c.Token, err = s.open(c.Secret)
		if err != nil {
			return c, err
		}
	}
	err = s.DB.WithContext(ctx).Where("credential_id = ?", id).Order("id").Find(&c.Regions).Error
	return c, err
}

type ListFilter struct {
	Page         int    `json:"page"`
	Size         int    `json:"size"`
	Keyword      string `json:"keyword"`
	Status       string `json:"status"`
	CredentialID uint   `json:"credential_id"`
}
type Page[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

func (f ListFilter) bounds() (int, int) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Size < 1 {
		f.Size = 20
	}
	if f.Size > 100 {
		f.Size = 100
	}
	return (f.Page - 1) * f.Size, f.Size
}
func (s *Store) Credentials(ctx context.Context, f ListFilter) (Page[Credential], error) {
	out := Page[Credential]{Items: []Credential{}}
	q := s.DB.WithContext(ctx).Model(&Credential{})
	keyword := strings.TrimSpace(f.Keyword)
	if len(keyword) > 2048 {
		return out, Invalid
	}
	if keyword != "" {
		if token, err := ParseToken(keyword); err == nil {
			// Use the indexed digest, never decrypt/scan tokens or mix an exact
			// credential lookup with fuzzy names that could match another user.
			q = q.Where("token_hash = ?", digest(token))
		} else if strings.Contains(keyword, "://") || strings.HasPrefix(keyword, "/d/") {
			// Incomplete/invalid links must not match similarly named records.
			q = q.Where("1 = 0")
		} else {
			q = q.Where("name LIKE ? OR batch LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
		}
	}
	switch f.Status {
	case "":
		q = q.Where("status <> ?", "revoked")
	case "enabled", "disabled", "revoked":
		q = q.Where("status = ?", f.Status)
	default:
		return out, Invalid
	}
	if err := q.Count(&out.Total).Error; err != nil {
		return out, err
	}
	offset, size := f.bounds()
	if err := q.Order("id DESC").Offset(offset).Limit(size).Find(&out.Items).Error; err != nil {
		return out, err
	}
	for i := range out.Items {
		var err error
		if out.Items[i].Status != "revoked" {
			out.Items[i].Token, err = s.open(out.Items[i].Secret)
			if err != nil {
				return out, err
			}
		}
		if err := s.DB.WithContext(ctx).Where("credential_id = ?", out.Items[i].ID).Order("id").Find(&out.Items[i].Regions).Error; err != nil {
			return out, err
		}
	}
	return out, nil
}
func (s *Store) Cards(ctx context.Context, f ListFilter) (Page[Card], error) {
	out := Page[Card]{Items: []Card{}}
	q := s.DB.WithContext(ctx).Model(&Card{})
	if f.Status == "enabled" || f.Status == "disabled" {
		q = q.Where("enabled = ?", f.Status == "enabled")
	} else if f.Status != "" {
		return out, Invalid
	}
	keyword := strings.TrimSpace(f.Keyword)
	if len(keyword) > 2048 {
		return out, Invalid
	}
	if keyword != "" {
		code := normalizeCardCode(keyword)
		if searchableCardCode.MatchString(code) {
			// Reuse redemption normalization and the indexed hash. Never decrypt
			// all cards or mix a precise code lookup with fuzzy name matches.
			q = q.Where("code_hash = ?", digest(code))
		} else {
			q = q.Where("name LIKE ? OR batch LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
		}
	}
	if err := q.Count(&out.Total).Error; err != nil {
		return out, err
	}
	offset, size := f.bounds()
	if err := q.Order("id DESC").Offset(offset).Limit(size).Find(&out.Items).Error; err != nil {
		return out, err
	}
	for i := range out.Items {
		var err error
		out.Items[i].Code, err = s.open(out.Items[i].Secret)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

type RedemptionSummary struct {
	Redemption
	CredentialName    string `json:"credential_name"`
	CredentialDeleted bool   `json:"credential_deleted"`
	CardName          string `json:"card_name"`
	CardDeleted       bool   `json:"card_deleted"`
}

func (s *Store) Redemptions(ctx context.Context, f ListFilter) (Page[RedemptionSummary], error) {
	out := Page[RedemptionSummary]{Items: []RedemptionSummary{}}
	q := s.DB.WithContext(ctx).Model(&Redemption{})
	if f.CredentialID > 0 {
		q = q.Where("credential_id = ?", f.CredentialID)
	}
	if err := q.Count(&out.Total).Error; err != nil {
		return out, err
	}
	offset, size := f.bounds()
	// Only names are joined, never credentials or codes. Keep deleted card names
	// for history; LEFT JOIN also retains records whose related row is missing.
	err := q.Select("distribution_redemptions.*, COALESCE(dc.name, '') AS credential_name, (dc.status = 'revoked') AS credential_deleted, COALESCE(card.name, '') AS card_name, (card.deleted_at IS NOT NULL) AS card_deleted").
		Joins("LEFT JOIN distribution_credentials dc ON dc.id = distribution_redemptions.credential_id").
		Joins("LEFT JOIN distribution_cards card ON card.id = distribution_redemptions.card_id").
		Order("distribution_redemptions.id DESC").Offset(offset).Limit(size).Find(&out.Items).Error
	return out, err
}
func (s *Store) Requests(ctx context.Context, f ListFilter) (Page[RegionRequest], error) {
	out := Page[RegionRequest]{Items: []RegionRequest{}}
	q := s.DB.WithContext(ctx).Model(&RegionRequest{})
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.CredentialID > 0 {
		q = q.Where("credential_id = ?", f.CredentialID)
	}
	if err := q.Count(&out.Total).Error; err != nil {
		return out, err
	}
	offset, size := f.bounds()
	err := q.Order("id DESC").Offset(offset).Limit(size).Find(&out.Items).Error
	return out, err
}
func (s *Store) Visits(ctx context.Context, f ListFilter) (Page[Visit], error) {
	out := Page[Visit]{Items: []Visit{}}
	q := s.DB.WithContext(ctx).Model(&Visit{})
	if f.CredentialID > 0 {
		q = q.Where("credential_id = ?", f.CredentialID)
	}
	if err := q.Count(&out.Total).Error; err != nil {
		return out, err
	}
	offset, size := f.bounds()
	err := q.Order("id DESC").Offset(offset).Limit(size).Find(&out.Items).Error
	return out, err
}
