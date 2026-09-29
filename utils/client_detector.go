package utils

import (
	"strings"
)

// ParseClientFromUA 根据 User-Agent 识别客户端名称
func ParseClientFromUA(ua string) string {
	if strings.TrimSpace(ua) == "" {
		return "未知客户端"
	}
	lower := strings.ToLower(ua)

	switch {
	case strings.Contains(lower, "bettbox"):
		return "Bettbox"
	case strings.Contains(lower, "flclash"):
		return "FlClash"
	case strings.Contains(lower, "clash-verge"):
		return "Clash Verge"
	case strings.Contains(lower, "clashmetaforandroid") || strings.Contains(lower, "cmfa"):
		return "CMFA"
	case strings.Contains(lower, "nekobox"):
		return "NekoBox"
	case strings.Contains(lower, "clash.meta") || strings.Contains(lower, "mihomo"):
		return "Mihomo"
	case strings.Contains(lower, "shadowrocket"):
		return "Shadowrocket"
	case strings.Contains(lower, "quantumult"):
		return "Quantumult X"
	case strings.Contains(lower, "surge"):
		return "Surge"
	case strings.Contains(lower, "loon"):
		return "Loon"
	case strings.Contains(lower, "stash"):
		return "Stash"
	case strings.Contains(lower, "sing-box") || strings.Contains(lower, "singbox"):
		return "sing-box"
	case strings.Contains(lower, "v2rayng"):
		return "v2rayNG"
	case strings.Contains(lower, "surfboard"):
		return "Surfboard"
	case strings.Contains(lower, "egern"):
		return "Egern"
	case strings.Contains(lower, "clash"):
		return "Clash"
	case strings.Contains(lower, "v2ray"):
		return "V2Ray"
	case strings.Contains(lower, "mozilla") || strings.Contains(lower, "chrome") || strings.Contains(lower, "safari"):
		return "浏览器"
	case strings.Contains(lower, "curl") || strings.Contains(lower, "wget") || strings.Contains(lower, "python") || strings.Contains(lower, "bot"):
		return "脚本/机器人"
	default:
		parts := strings.Split(ua, "/")
		if len(parts) > 0 && len(parts[0]) <= 20 {
			return parts[0]
		}
		if len(ua) > 20 {
			return ua[:20] + "..."
		}
		return ua
	}
}
