package distribution

import (
	"context"
	"net"
	"strings"
	"time"

	"gorm.io/gorm"
)

// AccessGrantInput is intentionally small: an exception is an administrator
// action for one exact public IP and a bounded lifetime, not a second access
// policy or a subnet allowlist.
type AccessGrantInput struct {
	IP        string    `json:"ip"`
	ExpiresAt time.Time `json:"expires_at"`
	Reason    string    `json:"reason"`
}

func normalizeGrantIP(value string) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return "", Invalid
	}
	return ip.String(), nil
}

func (s *Store) ListAccessGrants(ctx context.Context, credentialID uint) ([]AccessGrant, error) {
	if credentialID == 0 {
		return nil, Invalid
	}
	var credential Credential
	if err := s.DB.WithContext(ctx).First(&credential, credentialID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, NotFound
		}
		return nil, err
	}
	items := make([]AccessGrant, 0)
	err := s.DB.WithContext(ctx).Where("credential_id = ?", credentialID).Order("id DESC").Find(&items).Error
	return items, err
}

func (s *Store) CreateAccessGrant(ctx context.Context, credentialID uint, in AccessGrantInput) (AccessGrant, error) {
	var out AccessGrant
	now := s.Now().UTC()
	ip, err := normalizeGrantIP(in.IP)
	if err != nil || credentialID == 0 || in.ExpiresAt.IsZero() || !in.ExpiresAt.After(now) || len(in.Reason) > 500 {
		return out, Invalid
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = "管理员应急放行"
	}
	expires := in.ExpiresAt.UTC()
	err = s.transaction(ctx, func(tx *gorm.DB) error {
		var credential Credential
		if err := tx.First(&credential, credentialID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return NotFound
			}
			return err
		}
		if credential.Status == "revoked" {
			return Inactive
		}
		var existing AccessGrant
		if err := tx.Where("credential_id = ? AND ip = ? AND enabled = ? AND revoked_at IS NULL AND expires_at > ?", credentialID, ip, true, now).First(&existing).Error; err == nil {
			return Conflict
		} else if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		out = AccessGrant{CredentialID: credentialID, IP: ip, Reason: reason, ExpiresAt: expires, Enabled: true, CreatedAt: now}
		return tx.Create(&out).Error
	})
	return out, err
}

func (s *Store) RevokeAccessGrant(ctx context.Context, credentialID, grantID uint) error {
	if credentialID == 0 || grantID == 0 {
		return Invalid
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var grant AccessGrant
		if err := tx.Where("id = ? AND credential_id = ?", grantID, credentialID).First(&grant).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return NotFound
			}
			return err
		}
		if !grant.Enabled || grant.RevokedAt != nil {
			return nil
		}
		now := s.Now().UTC()
		return tx.Model(&grant).Updates(map[string]any{"enabled": false, "revoked_at": now}).Error
	})
}

func accessGrant(tx *gorm.DB, credentialID uint, ip string, now time.Time) (AccessGrant, bool, error) {
	if credentialID == 0 || strings.TrimSpace(ip) == "" {
		return AccessGrant{}, false, nil
	}
	normalized, err := normalizeGrantIP(ip)
	if err != nil {
		return AccessGrant{}, false, nil
	}
	var grant AccessGrant
	err = tx.Where("credential_id = ? AND ip = ? AND enabled = ? AND revoked_at IS NULL AND expires_at > ?", credentialID, normalized, true, now.UTC()).First(&grant).Error
	if err == gorm.ErrRecordNotFound {
		return AccessGrant{}, false, nil
	}
	if err != nil {
		return AccessGrant{}, false, err
	}
	return grant, true, nil
}
