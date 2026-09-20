package distribution

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct {
	DB  *gorm.DB
	Key string
	Now func() time.Time
}

const legacyDefaultAllowedUA = `(?i)^(?:clash|mihomo|shadowrocket|loon|surge|stash|quantumult|sing-box|v2rayng|v2rayn|hiddify|nekobox)(?:[ /\d._-]|$)`
const defaultAllowedUA = `(?i)^(?:clashmetaforandroid|clash|mihomo|shadowrocket|loon|surge|stash|quantumult|sing-box|v2rayng|v2rayn|hiddify|nekobox)(?:[ /\d._-]|$)`

func New(db *gorm.DB, key string) *Store { return &Store{DB: db, Key: key, Now: time.Now} }

func (s *Store) Migrate() error {
	if err := s.DB.AutoMigrate(&Settings{}, &Credential{}, &Region{}, &Card{}, &Redemption{}, &Visit{}, &RegionRequest{}); err != nil {
		return err
	}
	defaults := Settings{ID: 1, TrialDays: 15, FreeDays: 7, CycleDays: 7, RegionLimit: 2, CycleAnchor: time.Now().UTC().Truncate(24 * time.Hour),
		AllowedUA:      defaultAllowedUA,
		ExpiredMessage: "订阅已到期，请前往首页续订"}
	if err := s.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&defaults).Error; err != nil {
		return err
	}
	// Upgrade only the shipped default. Never broaden an administrator's custom
	// allowlist or replace any other saved business settings.
	return s.DB.Model(&Settings{}).Where("id = ? AND allowed_ua = ?", 1, legacyDefaultAllowedUA).
		UpdateColumn("allowed_ua", defaultAllowedUA).Error
}

// Lock the singleton before reading business rows. This gives SQLite, MySQL and
// PostgreSQL the same serialization semantics, including across app instances.
// No network calls or subscription rendering take place inside this transaction.
func (s *Store) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	for attempt := 0; attempt < 5; attempt++ {
		err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&Settings{}).Where("id = ?", 1).UpdateColumn("revision", gorm.Expr("revision + 1"))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("distribution not initialized")
			}
			return fn(tx)
		})
		if err == nil || (!strings.Contains(err.Error(), "database is locked") && !strings.Contains(err.Error(), "database table is locked")) {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return Conflict
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func (s *Store) seal(value string) (string, error) {
	if s.Key == "" {
		return "", errors.New("missing encryption key")
	}
	key := sha256.Sum256([]byte("distribution:" + s.Key))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(value), nil)), nil
}
func (s *Store) open(value string) (string, error) {
	key := sha256.Sum256([]byte("distribution:" + s.Key))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	data, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil || len(data) < aead.NonceSize() {
		return "", errors.New("invalid encrypted secret")
	}
	plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], nil)
	return string(plain), err
}

func (s *Store) Settings(ctx context.Context) (Settings, error) {
	var cfg Settings
	err := s.DB.WithContext(ctx).First(&cfg, 1).Error
	return cfg, err
}
func (s *Store) SaveSettings(ctx context.Context, cfg Settings) error {
	// Zero means omitted by an older API consumer; preserve the saved limit.
	if cfg.RegionLimit < 0 || cfg.RegionLimit > 2 {
		return Invalid
	}
	if cfg.TrialDays < 1 || cfg.TrialDays > 365 || cfg.FreeDays < 1 || cfg.FreeDays > 365 || cfg.CycleDays < 1 || cfg.CycleDays > 365 || cfg.CycleAnchor.IsZero() || len(cfg.AllowedUA) > 2000 || cfg.AllowedUA == "" || len(cfg.ExpiredMessage) > 200 || (cfg.ExpiredMessages == nil && cfg.ExpiredMessage == "") {
		return Invalid
	}
	if cfg.ExpiredMessages != nil {
		if len(cfg.ExpiredMessages) < 1 || len(cfg.ExpiredMessages) > 10 {
			return Invalid
		}
		for i, message := range cfg.ExpiredMessages {
			if !utf8.ValidString(message) || strings.IndexFunc(message, func(r rune) bool { return unicode.IsControl(r) || r == '\u2028' || r == '\u2029' }) >= 0 {
				return Invalid
			}
			message = strings.TrimSpace(message)
			if message == "" || utf8.RuneCountInString(message) > 200 {
				return Invalid
			}
			cfg.ExpiredMessages[i] = message
		}
	}
	if _, err := regexp.Compile(cfg.AllowedUA); err != nil {
		return Invalid
	}
	if cfg.PortalURL != "" {
		u, err := url.Parse(cfg.PortalURL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return Invalid
		}
	}
	if cfg.Domain != "" {
		if len(cfg.Domain) > 253 || strings.ContainsAny(cfg.Domain, "/:,\n\r\t *?#") {
			return Invalid
		}
		for _, part := range strings.Split(cfg.Domain, ".") {
			if !regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?$`).MatchString(part) {
				return Invalid
			}
		}
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		if cfg.RegionLimit > 0 {
			// Never silently remove existing authorizations when lowering the cap.
			var overLimit []uint
			if err := tx.Model(&Region{}).Select("distribution_regions.credential_id").
				Joins("JOIN distribution_credentials c ON c.id = distribution_regions.credential_id").
				Where("c.status <> ?", "revoked").Group("distribution_regions.credential_id").
				Having("COUNT(*) > ?", cfg.RegionLimit).Limit(1).Scan(&overLimit).Error; err != nil {
				return err
			}
			if len(overLimit) > 0 {
				return RegionLimitConflict
			}
		}
		if cfg.FallbackSubscriptionID < 0 {
			return Invalid
		}
		if cfg.FallbackSubscriptionID > 0 {
			if err := subscriptionExists(tx, cfg.FallbackSubscriptionID); err != nil {
				return err
			}
		}
		updates := map[string]any{"trial_days": cfg.TrialDays, "free_days": cfg.FreeDays, "cycle_days": cfg.CycleDays, "cycle_anchor": cfg.CycleAnchor, "portal_url": cfg.PortalURL, "domain": strings.ToLower(cfg.Domain), "allowed_ua": cfg.AllowedUA, "fallback_subscription_id": cfg.FallbackSubscriptionID}
		if cfg.RegionLimit > 0 {
			updates["region_limit"] = cfg.RegionLimit
		}
		if cfg.ExpiredMessage != "" {
			updates["expired_message"] = cfg.ExpiredMessage
		}
		// Older API consumers may omit the array; never erase an existing list.
		if cfg.ExpiredMessages != nil {
			encoded, err := json.Marshal(cfg.ExpiredMessages)
			if err != nil {
				return err
			}
			updates["expired_messages"] = string(encoded)
		}
		return tx.Model(&Settings{}).Where("id = ?", 1).Updates(updates).Error
	})
}
func subscriptionExists(tx *gorm.DB, id int) error {
	var n int64
	if err := tx.Table("subcriptions").Where("id = ? AND deleted_at IS NULL", id).Count(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return Invalid
	}
	return nil
}

type IssueInput struct {
	Name           string `json:"name"`
	Batch          string `json:"batch"`
	Count          int    `json:"count"`
	SubscriptionID int    `json:"subscription_id"`
}

func (s *Store) Issue(ctx context.Context, in IssueInput) ([]Credential, error) {
	if in.Count < 1 || in.Count > 100 || strings.TrimSpace(in.Name) == "" || len(in.Name) > 80 || len(in.Batch) > 100 || in.SubscriptionID < 1 {
		return nil, Invalid
	}
	var rows []Credential
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		rows = nil
		if err := subscriptionExists(tx, in.SubscriptionID); err != nil {
			return err
		}
		if in.Batch != "" {
			if err := tx.Where("batch = ?", in.Batch).Order("id").Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) > 0 {
				if len(rows) != in.Count || rows[0].SubscriptionID != in.SubscriptionID {
					return Conflict
				}
				for _, row := range rows {
					if row.Status == "revoked" {
						return Conflict
					}
				}
				return nil
			}
		}
		var cfg Settings
		if err := tx.First(&cfg, 1).Error; err != nil {
			return err
		}
		if in.Batch == "" {
			v, err := randomSecret()
			if err != nil {
				return err
			}
			in.Batch = v[:16]
		}
		for i := 0; i < in.Count; i++ {
			token, err := randomSecret()
			if err != nil {
				return err
			}
			secret, err := s.seal(token)
			if err != nil {
				return err
			}
			name := in.Name
			if in.Count > 1 {
				name = fmt.Sprintf("%s-%03d", in.Name, i+1)
			}
			row := Credential{Name: name, Batch: in.Batch, SubscriptionID: in.SubscriptionID, Secret: secret, TokenHash: digest(token), Status: "enabled", Plan: "trial", TrialDays: cfg.TrialDays, CreatedAt: s.Now(), UpdatedAt: s.Now()}
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
		rows[i].Token, err = s.open(rows[i].Secret)
		if err != nil {
			return nil, err
		}
	}
	return rows, nil
}

// ParseToken never downloads a user-provided URL. Only this module's path format
// or an exact 64-character token is accepted; legacy /c links cannot bypass it.
func ParseToken(input string) (string, error) {
	value := strings.TrimSpace(input)
	if len(value) > 2048 {
		return "", Invalid
	}
	if strings.Contains(value, "://") {
		u, err := url.Parse(value)
		if err != nil || u.User != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return "", Invalid
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) != 2 || parts[0] != "d" {
			return "", Invalid
		}
		value = parts[1]
	}
	if len(value) != 64 {
		return "", Invalid
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", Invalid
	}
	return strings.ToLower(value), nil
}
func credentialByToken(tx *gorm.DB, token string) (Credential, error) {
	var c Credential
	value, err := ParseToken(token)
	if err != nil {
		return c, NotFound
	}
	err = tx.Where("token_hash = ?", digest(value)).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = NotFound
	}
	return c, err
}
func (s *Store) Credential(ctx context.Context, token string) (Credential, error) {
	return credentialByToken(s.DB.WithContext(ctx), token)
}

type CredentialPatch struct {
	SubscriptionID *int       `json:"subscription_id"`
	Name           *string    `json:"name"`
	Status         *string    `json:"status"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Permanent      *bool      `json:"permanent"`
	Rotate         bool       `json:"rotate"`
}

func (s *Store) UpdateCredential(ctx context.Context, id uint, in CredentialPatch) (Credential, error) {
	var c Credential
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.First(&c, id).Error; err != nil {
			return NotFound
		}
		if c.Status == "revoked" {
			return Inactive
		}
		if in.SubscriptionID != nil {
			if *in.SubscriptionID < 1 {
				return Invalid
			}
			if err := subscriptionExists(tx, *in.SubscriptionID); err != nil {
				return err
			}
			c.SubscriptionID = *in.SubscriptionID
		}
		if in.Name != nil {
			if len(*in.Name) > 100 || strings.TrimSpace(*in.Name) == "" {
				return Invalid
			}
			c.Name = *in.Name
		}
		if in.Status != nil {
			switch *in.Status {
			case "enabled", "disabled", "revoked":
				c.Status = *in.Status
			default:
				return Invalid
			}
		}
		if in.ExpiresAt != nil || in.Permanent != nil {
			if c.ActivatedAt == nil {
				return NotActivated
			}
			if in.ExpiresAt != nil {
				c.ExpiresAt = in.ExpiresAt
				c.Permanent = false
			}
			if in.Permanent != nil {
				c.Permanent = *in.Permanent
			}
			if c.Permanent {
				c.ExpiresAt = nil
			} else if c.ExpiresAt == nil {
				return Invalid
			}
			c.Plan = "manual"
		}
		if in.Rotate {
			token, err := randomSecret()
			if err != nil {
				return err
			}
			secret, err := s.seal(token)
			if err != nil {
				return err
			}
			c.Secret = secret
			c.TokenHash = digest(token)
		}
		if err := tx.Save(&c).Error; err != nil {
			return err
		}
		return tx.Where("credential_id = ?", c.ID).Order("id").Find(&c.Regions).Error
	})
	if err == nil && c.Status != "revoked" {
		c.Token, err = s.open(c.Secret)
	}
	return c, err
}

// Deletion reuses the irreversible revoked state. Retain token hashes, paid
// card bindings and audit rows; serialize with delivery, renewal and edits.
func (s *Store) DeleteCredentials(ctx context.Context, ids []uint) error {
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
		if err := tx.Model(&Credential{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(ids)) {
			return NotFound
		}
		return tx.Model(&Credential{}).Where("id IN ? AND status <> ?", ids, "revoked").
			Updates(map[string]any{"status": "revoked", "updated_at": s.Now().UTC()}).Error
	})
}
