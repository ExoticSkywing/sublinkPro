package services

import (
	"fmt"
	"gorm.io/gorm"
	"sublink/services/distribution"
)

// The legacy importer remaps resource IDs and does not carry encrypted delivery
// credentials. Refuse before clearing anything instead of silently misbinding
// recipients. Full database + encryption-key backup/restore remains supported.
func ensureDistributionMigrationSafe(target, source *gorm.DB) error {
	for _, db := range []*gorm.DB{target, source} {
		for _, model := range []any{&distribution.Credential{}, &distribution.Card{}} {
			if !db.Migrator().HasTable(model) {
				continue
			}
			var count int64
			if err := db.Unscoped().Model(model).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return fmt.Errorf("源或目标数据库包含订阅分发数据，旧版导入工具无法安全迁移；请完整备份并恢复数据库与 API 加密密钥")
			}
		}
	}
	return nil
}
