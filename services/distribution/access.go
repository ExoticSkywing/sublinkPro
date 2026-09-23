package distribution

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Decision struct {
	Credential    Credential
	Result        string
	Emergency     bool `json:"emergency"`
	AccessGrantID uint `json:"-"`
}

func decisionWithIP(tx *gorm.DB, c Credential, city City, ip string, now time.Time) (Decision, error) {
	out := Decision{Credential: c, Result: "allowed"}
	if c.Status == "revoked" {
		out.Result = "revoked"
		return out, nil
	}
	if c.Status != "enabled" {
		out.Result = "inactive"
		return out, nil
	}
	grant, hasGrant, err := accessGrant(tx, c.ID, ip, now)
	if err != nil {
		return out, err
	}
	if city.Country == "" {
		if !c.Active(now) {
			out.Result = "location_unavailable"
			return out, nil
		}
		if hasGrant {
			out.Emergency = true
			out.AccessGrantID = grant.ID
			return out, nil
		}
		out.Result = "location_unavailable"
		return out, nil
	}
	if city.Country != "CN" {
		out.Result = "country_denied"
		return out, nil
	}
	if city.Key == "" || city.Name == "" {
		if !c.Active(now) {
			out.Result = "location_unavailable"
			return out, nil
		}
		if hasGrant {
			out.Emergency = true
			out.AccessGrantID = grant.ID
			return out, nil
		}
		out.Result = "location_unavailable"
		return out, nil
	}
	var regions []Region
	if err := tx.Where("credential_id = ?", c.ID).Order("id").Find(&regions).Error; err != nil {
		return out, err
	}
	out.Credential.Regions = regions
	known := false
	for _, r := range regions {
		if r.CityKey == city.Key {
			known = true
		}
	}
	var cfg Settings
	if err := tx.First(&cfg, 1).Error; err != nil {
		return out, err
	}
	if !known && len(regions) >= cfg.RegionLimit {
		if !c.Active(now) {
			out.Result = "region_denied"
			return out, nil
		}
		if hasGrant {
			out.Emergency = true
			out.AccessGrantID = grant.ID
			return out, nil
		}
		out.Result = "region_denied"
	}
	if !c.Active(now) {
		out.Result = "expired"
	}
	return out, nil
}

func decision(tx *gorm.DB, c Credential, city City, now time.Time) (Decision, error) {
	return decisionWithIP(tx, c, city, "", now)
}
func (s *Store) Check(ctx context.Context, token string, city City, ips ...string) (Decision, error) {
	tx := s.DB.WithContext(ctx)
	c, err := credentialByToken(tx, token)
	if err != nil {
		return Decision{}, err
	}
	ip := ""
	if len(ips) > 0 {
		ip = ips[0]
	}
	return decisionWithIP(tx, c, city, ip, s.Now())
}

// CommitDelivery must only be called after a valid nonempty subscription has
// been rendered. It rechecks permissions to close races with revocation, expiry,
// simultaneous first activation, resource changes, and changes to the city limit.
func (s *Store) CommitDelivery(ctx context.Context, token string, city City, subscriptionID int, ips ...string) (Decision, error) {
	var out Decision
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		c, err := credentialByToken(tx, token)
		if err != nil {
			return err
		}
		if c.SubscriptionID != subscriptionID {
			return Conflict // Discard a response rendered from the previous resource pool.
		}
		now := s.Now().UTC()
		ip := ""
		if len(ips) > 0 {
			ip = ips[0]
		}
		out, err = decisionWithIP(tx, c, city, ip, now)
		if err != nil || out.Result != "allowed" {
			return err
		}
		if c.ActivatedAt == nil {
			expires := now.Add(time.Duration(c.TrialDays) * 24 * time.Hour)
			c.ActivatedAt = &now
			c.ExpiresAt = &expires
		}
		known := false
		for _, r := range out.Credential.Regions {
			if r.CityKey == city.Key {
				known = true
			}
		}
		if !out.Emergency && city.Key != "" && city.Name != "" && !known {
			row := Region{CredentialID: c.ID, CityKey: city.Key, Province: city.Province, City: city.Name, CreatedAt: now}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			out.Credential.Regions = append(out.Credential.Regions, row)
		}
		c.LastAccessAt = &now
		c.AccessCount++
		c.Regions = out.Credential.Regions
		if out.Emergency {
			if err := tx.Model(&AccessGrant{}).Where("id = ? AND credential_id = ?", out.AccessGrantID, c.ID).Updates(map[string]any{"last_used_at": now}).Error; err != nil {
				return err
			}
		}
		if err := tx.Save(&c).Error; err != nil {
			return err
		}
		out.Credential = c
		return nil
	})
	return out, err
}

func (s *Store) RecordVisit(ctx context.Context, v Visit) error {
	if len(v.UA) > 512 {
		v.UA = v.UA[:512]
	}
	v.CreatedAt = s.Now().UTC()
	if err := s.DB.WithContext(ctx).Create(&v).Error; err != nil {
		return err
	}
	// Keep a bounded recent history per credential; approval requests copy their
	// target location, so pruning a visit never changes an existing application.
	var cutoff Visit
	err := s.DB.WithContext(ctx).Where("credential_id = ?", v.CredentialID).Order("id DESC").Offset(199).First(&cutoff).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Where("credential_id = ? AND id < ?", v.CredentialID, cutoff.ID).Delete(&Visit{}).Error
}

type PublicStatus struct {
	ID          uint            `json:"id"`
	Status      string          `json:"status"`
	Plan        string          `json:"plan"`
	ActivatedAt *time.Time      `json:"activated_at"`
	ExpiresAt   *time.Time      `json:"expires_at"`
	Permanent   bool            `json:"permanent"`
	Regions     []Region        `json:"regions"`
	RegionLimit int             `json:"region_limit"`
	Candidates  []Visit         `json:"candidates"`
	Requests    []RegionRequest `json:"requests"`
}

func (s *Store) PublicStatus(ctx context.Context, token string) (PublicStatus, error) {
	tx := s.DB.WithContext(ctx)
	c, err := credentialByToken(tx, token)
	if err != nil {
		return PublicStatus{}, err
	}
	out := PublicStatus{ID: c.ID, Status: c.Status, Plan: c.Plan, ActivatedAt: c.ActivatedAt, ExpiresAt: c.ExpiresAt, Permanent: c.Permanent, Regions: []Region{}, Candidates: []Visit{}, Requests: []RegionRequest{}}
	var cfg Settings
	if err := tx.First(&cfg, 1).Error; err != nil {
		return out, err
	}
	out.RegionLimit = cfg.RegionLimit
	if err := tx.Where("credential_id = ?", c.ID).Order("id").Find(&out.Regions).Error; err != nil {
		return out, err
	}
	if err := tx.Select("id", "city_key", "province", "city", "created_at").Where("credential_id = ? AND result = ? AND created_at >= ?", c.ID, "region_denied", s.Now().Add(-7*24*time.Hour)).Order("id DESC").Limit(30).Find(&out.Candidates).Error; err != nil {
		return out, err
	}
	seen := map[string]bool{}
	for _, region := range out.Regions {
		seen[region.CityKey] = true
	}
	unique := []Visit{}
	for _, v := range out.Candidates {
		if !seen[v.CityKey] {
			seen[v.CityKey] = true
			unique = append(unique, v)
		}
	}
	out.Candidates = unique
	err = tx.Where("credential_id = ?", c.ID).Order("id DESC").Limit(20).Find(&out.Requests).Error
	return out, err
}

type ApplyInput struct {
	Link    string `json:"link"`
	VisitID uint   `json:"visit_id"`
	Reason  string `json:"reason"`
	Contact string `json:"contact"`
}

func (s *Store) ApplyRegion(ctx context.Context, in ApplyInput) (RegionRequest, error) {
	var out RegionRequest
	if len(strings.TrimSpace(in.Reason)) < 3 || len(in.Reason) > 1000 || len(in.Contact) > 200 || in.VisitID == 0 {
		return out, Invalid
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		out = RegionRequest{}
		c, err := credentialByToken(tx, in.Link)
		if err != nil {
			return err
		}
		if c.Status != "enabled" {
			return Inactive
		}
		if c.ActivatedAt == nil {
			return NotActivated
		}
		var v Visit
		if err := tx.Where("id = ? AND credential_id = ? AND result = ? AND country = ? AND created_at >= ?", in.VisitID, c.ID, "region_denied", "CN", s.Now().Add(-7*24*time.Hour)).First(&v).Error; err != nil {
			return Invalid
		}
		var count int64
		if err := tx.Model(&Region{}).Where("credential_id = ? AND city_key = ?", c.ID, v.CityKey).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return Conflict
		}
		err = tx.Where("credential_id = ? AND status = ?", c.ID, "pending").First(&out).Error
		if err == nil {
			if out.CityKey == v.CityKey {
				return nil
			}
			return Conflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Model(&RegionRequest{}).Where("credential_id = ? AND created_at >= ?", c.ID, s.Now().Add(-24*time.Hour)).Count(&count).Error; err != nil {
			return err
		}
		if count >= 3 {
			return TooFrequent
		}
		out = RegionRequest{CredentialID: c.ID, VisitID: v.ID, CityKey: v.CityKey, Province: v.Province, City: v.City, Reason: strings.TrimSpace(in.Reason), Contact: in.Contact, Status: "pending", CreatedAt: s.Now()}
		return tx.Create(&out).Error
	})
	return out, err
}

type ReviewInput struct {
	Approve         bool   `json:"approve"`
	ReplaceRegionID uint   `json:"replace_region_id"`
	Reply           string `json:"reply"`
}

func (s *Store) ReviewRegion(ctx context.Context, id uint, in ReviewInput, admin string) error {
	if len(in.Reply) > 1000 || (!in.Approve && strings.TrimSpace(in.Reply) == "") {
		return Invalid
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var req RegionRequest
		if err := tx.First(&req, id).Error; err != nil {
			return NotFound
		}
		if req.Status != "pending" {
			return Conflict
		}
		var c Credential
		if err := tx.First(&c, req.CredentialID).Error; err != nil {
			return NotFound
		}
		if c.Status != "enabled" {
			return Inactive
		}
		if in.Approve {
			var regions []Region
			if err := tx.Where("credential_id = ?", c.ID).Find(&regions).Error; err != nil {
				return err
			}
			for _, r := range regions {
				if r.CityKey == req.CityKey {
					return Conflict
				}
			}
			var cfg Settings
			if err := tx.First(&cfg, 1).Error; err != nil {
				return err
			}
			if len(regions) >= cfg.RegionLimit {
				found := false
				for _, r := range regions {
					if r.ID == in.ReplaceRegionID {
						found = true
					}
				}
				if !found {
					return Invalid
				}
				if err := tx.Where("id = ? AND credential_id = ?", in.ReplaceRegionID, c.ID).Delete(&Region{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Create(&Region{CredentialID: c.ID, CityKey: req.CityKey, Province: req.Province, City: req.City, CreatedAt: s.Now()}).Error; err != nil {
				return err
			}
			req.Status = "approved"
		} else {
			req.Status = "rejected"
		}
		now := s.Now()
		req.ReviewedAt = &now
		req.ReviewedBy = admin
		req.Reply = in.Reply
		return tx.Save(&req).Error
	})
}
