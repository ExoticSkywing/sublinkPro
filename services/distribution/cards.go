package distribution

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type CardInput struct {
	Name   string     `json:"name"`
	Batch  string     `json:"batch"`
	Count  int        `json:"count"`
	Kind   string     `json:"kind"`
	EndsAt *time.Time `json:"ends_at"`
}

// 80 random bits, displayed as four groups. Link tokens keep their separate,
// longer generator. Exclude I/O to make manual transcription less ambiguous.
func randomCardCode() (string, error) {
	var data [10]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	code := base32.NewEncoding("23456789ABCDEFGHJKLMNPQRSTUVWXYZ").WithPadding(base32.NoPadding).EncodeToString(data[:])
	return code[:4] + "-" + code[4:8] + "-" + code[8:12] + "-" + code[12:], nil
}

func normalizeCardCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	compact := strings.NewReplacer("-", "", " ", "", "\t", "", "\r", "", "\n", "").Replace(code)
	if len(compact) == 16 {
		return compact
	}
	// Existing 64-character codes retain their original lookup semantics.
	return code
}

func (s *Store) CreateCards(ctx context.Context, in CardInput) ([]Card, error) {
	if in.Count < 1 || in.Count > 100 || len(in.Name) > 80 || strings.TrimSpace(in.Name) == "" || len(in.Batch) > 100 {
		return nil, Invalid
	}
	if in.Kind != "quarter" && in.Kind != "year" && in.Kind != "permanent" {
		return nil, Invalid
	}
	if in.EndsAt != nil && !in.EndsAt.After(s.Now()) {
		return nil, Invalid
	}
	var rows []Card
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		rows = nil
		if in.Batch != "" {
			if err := tx.Unscoped().Where("batch = ?", in.Batch).Order("id").Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) > 0 {
				for _, row := range rows {
					if row.DeletedAt.Valid || row.Kind != in.Kind {
						return Conflict
					}
				}
				if len(rows) != in.Count || rows[0].Kind != in.Kind {
					return Conflict
				}
				return nil
			}
		}
		if in.Batch == "" {
			v, err := randomSecret()
			if err != nil {
				return err
			}
			in.Batch = v[:16]
		}
		for i := 0; i < in.Count; i++ {
			code, err := randomCardCode()
			if err != nil {
				return err
			}
			secret, err := s.seal(code)
			if err != nil {
				return err
			}
			name := in.Name
			if in.Count > 1 {
				name = fmt.Sprintf("%s-%03d", in.Name, i+1)
			}
			row := Card{Name: name, Batch: in.Batch, Kind: in.Kind, Secret: secret, CodeHash: digest(normalizeCardCode(code)), Enabled: true, StartsAt: s.Now(), EndsAt: in.EndsAt, CreatedAt: s.Now()}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			rows = append(rows, row)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Code, err = s.open(rows[i].Secret)
		if err != nil {
			return nil, err
		}
	}
	return rows, nil
}

// CurrentFreeCard is an idempotent, on-demand rotation API for both the admin UI
// and external delivery systems. A disabled code is not silently replaced.
func (s *Store) CurrentFreeCard(ctx context.Context) (Card, error) {
	return s.currentFreeCard(ctx, false)
}

// ReissueFreeCard explicitly replaces a deleted current-cycle code. Repeated
// calls return the live code; disabled codes are never replaced or re-enabled.
func (s *Store) ReissueFreeCard(ctx context.Context) (Card, error) {
	return s.currentFreeCard(ctx, true)
}

func (s *Store) currentFreeCard(ctx context.Context, reissue bool) (Card, error) {
	var card Card
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		card = Card{}
		var cfg Settings
		if err := tx.First(&cfg, 1).Error; err != nil {
			return err
		}
		now := s.Now().UTC()
		if now.Before(cfg.CycleAnchor) {
			return Invalid
		}
		period := time.Duration(cfg.CycleDays) * 24 * time.Hour
		start := cfg.CycleAnchor.Add((now.Sub(cfg.CycleAnchor) / period) * period)
		end := start.Add(period)
		cycle := fmt.Sprintf("free:%d", start.Unix())
		err := tx.Unscoped().Where("cycle = ?", cycle).Order("id DESC").First(&card).Error
		if err == nil {
			if !card.DeletedAt.Valid {
				return nil
			}
			if !reissue {
				return CardDeleted
			}
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		code, err := randomCardCode()
		if err != nil {
			return err
		}
		secret, err := s.seal(code)
		if err != nil {
			return err
		}
		card = Card{Name: start.Format("2006-01-02") + " 免费续期", Batch: cycle, Kind: "free", Cycle: cycle, Days: cfg.FreeDays, Secret: secret, CodeHash: digest(normalizeCardCode(code)), Enabled: true, StartsAt: start, EndsAt: &end, CreatedAt: now}
		return tx.Create(&card).Error
	})
	if err == nil {
		card.Code, err = s.open(card.Secret)
	}
	return card, err
}
func (s *Store) SetCardEnabled(ctx context.Context, id uint, enabled bool) error {
	return s.UpdateCard(ctx, id, CardPatch{Enabled: &enabled})
}

// RawMessage distinguishes omitted deadline (unchanged) from null (clear).
type CardPatch struct {
	Name    *string         `json:"name"`
	Kind    *string         `json:"kind"`
	EndsAt  json.RawMessage `json:"ends_at"`
	Enabled *bool           `json:"enabled"`
}

func (s *Store) UpdateCard(ctx context.Context, id uint, in CardPatch) error {
	if id == 0 || (in.Name == nil && in.Kind == nil && len(in.EndsAt) == 0 && in.Enabled == nil) {
		return Invalid
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var card Card
		if err := tx.First(&card, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return NotFound
			}
			return err
		}
		if in.Name != nil {
			name := strings.TrimSpace(*in.Name)
			if name == "" || len(name) > 100 {
				return Invalid
			}
			card.Name = name
		}
		if in.Kind != nil && *in.Kind != card.Kind {
			if card.Kind == "free" || card.RedemptionCount > 0 || card.BoundCredentialID != nil {
				return CardLocked
			}
			if *in.Kind != "quarter" && *in.Kind != "year" && *in.Kind != "permanent" {
				return Invalid
			}
			card.Kind = *in.Kind
		}
		if len(in.EndsAt) > 0 {
			var end *time.Time
			if err := json.Unmarshal(in.EndsAt, &end); err != nil {
				return Invalid
			}
			same := (end == nil && card.EndsAt == nil) || (end != nil && card.EndsAt != nil && end.Equal(*card.EndsAt))
			if !same {
				if card.Kind == "free" {
					return CardLocked
				}
				if end != nil && !end.After(s.Now()) {
					return Invalid
				}
				card.EndsAt = end
			}
		}
		if in.Enabled != nil {
			card.Enabled = *in.Enabled
		}
		return tx.Save(&card).Error
	})
}

// Keep tombstones and redemption history, and invalidate the code immediately.
// The transaction serializes deletion with redemption and supports atomic batches.
func (s *Store) DeleteCards(ctx context.Context, ids []uint) error {
	if len(ids) < 1 || len(ids) > 100 {
		return Invalid
	}
	seen := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			return Invalid
		}
		seen[id] = true
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Unscoped().Model(&Card{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(ids)) {
			return NotFound
		}
		// Explicit timestamp uses the store clock and makes retries harmless.
		return tx.Model(&Card{}).Where("id IN ?", ids).Updates(map[string]any{"enabled": false, "deleted_at": s.Now()}).Error
	})
}

// Calendar months clamp month-end dates instead of overflowing into the next
// month (e.g. Jan 31 + 3 months is Apr 30, not May 1).
func addMonths(t time.Time, months int) time.Time {
	base := time.Date(t.Year(), t.Month()+time.Month(months), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	last := base.AddDate(0, 1, -1).Day()
	day := t.Day()
	if day > last {
		day = last
	}
	return base.AddDate(0, 0, day-1)
}

func (s *Store) Redeem(ctx context.Context, token, code, source string) (Redemption, error) {
	var out Redemption
	if len(code) > 128 {
		return out, CardUnavailable
	}
	code = normalizeCardCode(code)
	if source != "web" && source != "api" {
		return out, Invalid
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		out = Redemption{}
		c, err := credentialByToken(tx, token)
		if err != nil {
			return err
		}
		if c.Status != "enabled" {
			return Inactive
		}
		if c.ActivatedAt == nil {
			return NotActivated
		}
		var card Card
		if err := tx.Where("code_hash = ?", digest(code)).First(&card).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return CardUnavailable
			}
			return err
		}
		benefit := fmt.Sprintf("card:%d", card.ID)
		if card.Kind == "free" {
			benefit = card.Cycle
		}
		err = tx.Where("credential_id = ? AND benefit_key = ?", c.ID, benefit).First(&out).Error
		if err == nil {
			out.AlreadyRedeemed = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := s.Now().UTC()
		if !card.Enabled || now.Before(card.StartsAt) || (card.EndsAt != nil && !now.Before(*card.EndsAt)) {
			return CardUnavailable
		}
		if card.BoundCredentialID != nil && *card.BoundCredentialID != c.ID {
			return CardUsed
		}
		if c.Permanent {
			return Covered
		}
		before := c.ExpiresAt
		base := now
		if before != nil && before.After(now) {
			base = *before
		}
		var after time.Time
		switch card.Kind {
		case "free":
			if c.Plan != "free" && before != nil && before.After(now) {
				return Covered
			}
			after = now.Add(time.Duration(card.Days) * 24 * time.Hour)
			if before != nil && !after.After(*before) {
				return Covered
			}
		case "quarter":
			after = addMonths(base, 3)
		case "year":
			after = addMonths(base, 12)
		case "permanent":
			c.Permanent = true
		default:
			return CardUnavailable
		}
		if c.Permanent {
			c.ExpiresAt = nil
		} else {
			c.ExpiresAt = &after
		}
		c.Plan = card.Kind
		if err := tx.Save(&c).Error; err != nil {
			return err
		}
		if card.Kind != "free" {
			card.BoundCredentialID = &c.ID
		}
		card.RedemptionCount++
		if err := tx.Save(&card).Error; err != nil {
			return err
		}
		out = Redemption{CredentialID: c.ID, BenefitKey: benefit, CardID: card.ID, Kind: card.Kind, Before: before, After: c.ExpiresAt, Permanent: c.Permanent, Source: source, CreatedAt: now}
		return tx.Create(&out).Error
	})
	return out, err
}
