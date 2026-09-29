package utils

import (
	"testing"
)

func TestParseClientFromUA(t *testing.T) {
	tests := []struct {
		ua   string
		want string
	}{
		{"clash-verge/v2.5.2", "Clash Verge"},
		{"ClashMetaForAndroid/2.11.25.Meta", "Clash Meta (Android)"},
		{"FlClash/ClashMetaForAndroid/2.11.33.Bettbox", "Bettbox"},
		{"FlClash/v0.8.98", "FlClash"},
		{"Shadowrocket/3445 CFNetwork/1402.0.8", "Shadowrocket"},
		{"Loon/998", "Loon"},
		{"TelegramBot (like TwitterBot)", "脚本/机器人"},
		{"", "未知客户端"},
	}

	for _, tt := range tests {
		t.Run(tt.ua, func(t *testing.T) {
			if got := ParseClientFromUA(tt.ua); got != tt.want {
				t.Errorf("ParseClientFromUA(%q) = %q, want %q", tt.ua, got, tt.want)
			}
		})
	}
}
