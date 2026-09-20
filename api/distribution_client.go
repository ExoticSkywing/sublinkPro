package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"

	"sublink/config"
	"sublink/models"
	"sublink/node/protocol"
	"sublink/services/distribution"
	"sublink/services/geoip"
	"sublink/utils"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// Buffer before activation and perform a final authorization check before any
// resource bytes leave the process. An error, empty conversion or oversized
// output cannot activate a trial or reserve a city.
type distributionBuffer struct {
	gin.ResponseWriter
	headers http.Header
	body    bytes.Buffer
	status  int
	written bool
	failed  bool
}

func (w *distributionBuffer) Header() http.Header { return w.headers }
func (w *distributionBuffer) WriteHeader(code int) {
	if !w.written {
		w.status = code
	}
}
func (w *distributionBuffer) WriteHeaderNow() { w.written = true }
func (w *distributionBuffer) Write(b []byte) (int, error) {
	w.written = true
	if w.body.Len()+len(b) > 16<<20 {
		w.failed = true
		return 0, errors.New("subscription exceeds size limit")
	}
	return w.body.Write(b)
}
func (w *distributionBuffer) WriteString(v string) (int, error) { return w.Write([]byte(v)) }
func (w *distributionBuffer) Status() int                       { return w.status }
func (w *distributionBuffer) Size() int {
	if !w.written {
		return -1
	}
	return w.body.Len()
}
func (w *distributionBuffer) Written() bool { return w.written }
func (w *distributionBuffer) Flush()        { w.written = true }

func distributionPortal(cfg distribution.Settings) string {
	if cfg.PortalURL != "" {
		return cfg.PortalURL
	}
	return strings.TrimRight(config.GetWebBasePath(), "/") + "/"
}
func distributionClientType(c *gin.Context) string {
	if c.Query("client") != "" {
		return normalizeDistributionClient(resolveSubscriptionClient(c))
	}
	ua := strings.ToLower(c.GetHeader("User-Agent"))
	for _, name := range []string{"shadowrocket", "loon", "stash", "quantumult", "surge", "mihomo", "clash", "sing-box"} {
		if strings.HasPrefix(ua, name) {
			switch name {
			case "shadowrocket", "loon":
				// Like legacy auto shares, these clients accept the native URI
				// list. Only an explicit client=loon requests Sub-Store conversion.
				return "v2ray"
			case "quantumult":
				return "quanx"
			default:
				return name
			}
		}
	}
	return "v2ray"
}
func normalizeDistributionClient(client string) string {
	switch strings.ToLower(strings.TrimSpace(client)) {
	case "clashmeta", "clash-meta":
		return "mihomo"
	case "singbox":
		return "sing-box"
	case "qx", "quantumultx", "quantumult-x":
		return "quanx"
	case "v2ray-uri", "v2rayuri":
		return "uri"
	default:
		return strings.ToLower(strings.TrimSpace(client))
	}
}
func distributionPrepared(c *gin.Context, client string, subscriptionID int) (preparedClientResponse, bool) {
	sub := models.Subcription{ID: subscriptionID}
	if err := sub.Find(); err != nil {
		return preparedClientResponse{}, false
	}
	if (sub.IPBlacklist != "" && utils.IsIpInCidr(c.ClientIP(), sub.IPBlacklist)) || (sub.IPWhitelist != "" && !utils.IsIpInCidr(c.ClientIP(), sub.IPWhitelist)) {
		return preparedClientResponse{}, false
	}
	prepared, ok := buildPreparedResponseFromSubscription(sub, client, 0)
	if !ok || len(prepared.Subscription.Nodes) == 0 {
		return preparedClientResponse{}, false
	}
	prepared.Subscription.UpdateInterval = 24
	return prepared, true
}
func distributionNotice(c *gin.Context, cfg distribution.Settings, client, name, message string, expired bool) {
	portal := distributionPortal(cfg)
	if strings.HasPrefix(portal, "/") {
		portal = "https://" + c.Request.Host + portal
	}
	// Each built-in sentence and the portal occupy their own notice node, so
	// narrow client cards do not hide the recovery action behind a long label.
	messages := append(strings.Split(message, "，"), portal)
	if expired {
		messages = cfg.ExpiredMessages
		if len(messages) == 0 {
			// Preserve legacy copy while separating its formerly appended URL.
			messages = []string{cfg.ExpiredMessage, "{portal}"}
		}
	}
	prepared := buildSyntheticFallbackResponse(client, messages[0])
	// Keep the synthetic template, but prevent the shared renderer from
	// replacing the list with its single-node fallback.
	prepared.Mode = clientResponseNormal
	prepared.Subscription.Nodes = nil
	usedNames := make(map[string]bool, len(messages))
	for i, text := range messages {
		if expired {
			text = strings.ReplaceAll(text, "{portal}", portal)
		}
		base := text
		for suffix := 2; usedNames[text]; suffix++ {
			text = fmt.Sprintf("%s (%d)", base, suffix)
		}
		usedNames[text] = true
		prepared.Subscription.Nodes = append(prepared.Subscription.Nodes, models.Node{
			ID: -(i + 1), Name: text, LinkName: text, Protocol: "ss", Source: "manual",
			// Shared renderers treat literal commas as multi-link separators.
			Link: strings.ReplaceAll(protocol.EncodeSSURL(protocol.Ss{Name: text, Server: "placeholder.invalid", Port: 80,
				Param: protocol.Param{Cipher: "aes-128-gcm", Password: "placeholder"}}), ",", "%2C"),
		})
	}
	setDistributionName(&prepared, name)
	if expired && cfg.FallbackSubscriptionID > 0 {
		if fallback, ok := distributionPrepared(c, client, cfg.FallbackSubscriptionID); ok {
			// Only the selected fallback nodes enter a fresh synthetic envelope;
			// templates, scripts and remote includes cannot reintroduce full nodes.
			for _, n := range fallback.Subscription.Nodes {
				if n.Protocol != "" && n.Protocol != "http" && n.Protocol != "https" {
					prepared.Subscription.Nodes = append(prepared.Subscription.Nodes, n)
				}
			}
		}
	}
	prepared.Subscription.UpdateInterval = 24
	buffer := renderDistribution(c, prepared)
	body, err := distributionBodyPolicy(c, buffer.body.Bytes(), client, cfg)
	if err != nil || buffer.failed || buffer.status != http.StatusOK {
		c.String(http.StatusServiceUnavailable, "Subscription notice generation failed; please retry")
		return
	}
	for key, values := range buffer.headers {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	c.Header("Cache-Control", "no-store, private")
	c.Header("profile-update-interval", "24")
	_, _ = c.Writer.Write(body)
}

// Errors belong in notice nodes, never in the profile's persistent identity.
func setDistributionName(prepared *preparedClientResponse, name string) {
	if strings.TrimSpace(name) == "" {
		name = "订阅"
	}
	prepared.SubName = name
	prepared.Subscription.Name = name
	prepared.FallbackIdentity = fallbackIdentityOriginalEnvelope
}

func renderDistribution(c *gin.Context, prepared preparedClientResponse) *distributionBuffer {
	original := c.Writer
	buffer := &distributionBuffer{ResponseWriter: original, headers: make(http.Header), status: http.StatusOK}
	c.Writer = buffer
	defer func() { c.Writer = original }()
	setResolvedSubscriptionName(c, prepared.SubName)
	dispatchPreparedClientResponse(c, prepared)
	// Follow the resource subscription's usage switch. The shared renderer also
	// sends cached/zero usage when disabled, so remove it at this boundary only.
	// Synthetic notices have the switch off and never expose resource usage.
	if !prepared.Subscription.RefreshUsageOnRequest {
		buffer.headers.Del("subscription-userinfo")
	}
	buffer.headers.Set("profile-title", "base64:"+base64.StdEncoding.EncodeToString([]byte(prepared.SubName)))
	if _, params, err := mime.ParseMediaType(buffer.headers.Get("Content-Disposition")); err == nil {
		// RFC 5987 encoding preserves spaces and literal plus signs in client names.
		filename := prepared.SubName + path.Ext(params["filename"])
		buffer.headers.Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filename}))
	}
	return buffer
}

func distributionBodyPolicy(c *gin.Context, body []byte, client string, cfg distribution.Settings) ([]byte, error) {
	body, err := distributionDirectRule(body, client, cfg.Domain)
	if err != nil {
		return nil, err
	}
	if client == "surge" || client == "surfboard" {
		// Renewal URLs can point off-site; never send subscription secrets there.
		origin := "https://" + c.Request.Host
		if cfg.Domain != "" {
			origin = "https://" + cfg.Domain
		}
		managed := "#!MANAGED-CONFIG " + origin + c.Request.URL.RequestURI() + " interval=86400 strict=false"
		body = []byte(managed + "\n" + regexp.MustCompile(`(?im)^#!MANAGED-CONFIG[^\r\n]*[\r\n]*`).ReplaceAllString(string(body), ""))
	}
	return body, nil
}

func distributionDecisionMessage(result string, city distribution.City, cfg distribution.Settings) string {
	switch result {
	case "expired":
		return cfg.ExpiredMessage
	case "inactive":
		return "订阅已停用，请联系管理员恢复"
	case "revoked":
		return "订阅链接已失效，请联系管理员"
	case "country_denied":
		return "仅允许中国大陆 IP 拉取，请将订阅更新设为直连"
	case "region_denied":
		return "当前地区「" + city.Province + "·" + city.Name + "」未授权，请到首页申请调整"
	case "location_unavailable":
		return "暂时无法识别所在城市，请稍后重试或联系管理员"
	default:
		return "无效的订阅链接"
	}
}

func DistributionClient(c *gin.Context) {
	distributionClient(c, distributionStore(), func(ip string) (geoip.CityLocation, error) {
		return geoip.GetCityLocationContext(c.Request.Context(), ip)
	})
}

// Dependencies are arguments so HTTP tests can use isolated data and locations.
// Production uses the trusted client IP and the bounded city fallback chain.
func distributionClient(c *gin.Context, store *distribution.Store, locate func(string) (geoip.CityLocation, error)) {
	cfg, err := store.Settings(c.Request.Context())
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	ua := c.GetHeader("User-Agent")
	if strings.HasPrefix(strings.ToLower(ua), "mozilla/") {
		c.Redirect(http.StatusFound, distributionPortal(cfg))
		return
	}
	matcher, err := regexp.Compile(cfg.AllowedUA)
	if err != nil || len(ua) > 512 || ua == "" || !matcher.MatchString(ua) {
		if cred, e := store.Credential(c.Request.Context(), c.Param("token")); e == nil {
			_ = store.RecordVisit(c.Request.Context(), distribution.Visit{CredentialID: cred.ID, IP: c.ClientIP(), UA: ua, Result: "ua_denied"})
		}
		c.String(http.StatusForbidden, "Unsupported subscription client")
		return
	}
	client := distributionClientType(c)
	// Invalid links and HEAD probes must not spend third-party API quota.
	cred, credentialErr := store.Credential(c.Request.Context(), c.Param("token"))
	if c.Request.Method == http.MethodHead {
		if credentialErr != nil {
			c.Status(http.StatusNotFound)
		} else {
			c.Status(http.StatusOK)
		}
		return
	}
	if credentialErr != nil {
		distributionNotice(c, cfg, client, "订阅", "无效的订阅链接", false)
		return
	}
	var location geoip.CityLocation
	var geoErr error
	if cred.Status == "enabled" {
		location, geoErr = locate(c.ClientIP())
	}
	city := distribution.City{Key: location.Key, Country: location.Country, Province: location.Province, Name: location.City}
	if geoErr != nil {
		city.Key = "" // Keep known country/province for diagnostics, never authorize.
	}
	result := "generation_failed"
	var credentialID uint
	defer func() {
		if credentialID > 0 {
			if err := store.RecordVisit(c.Request.Context(), distribution.Visit{CredentialID: credentialID, IP: c.ClientIP(), UA: ua, Country: city.Country, CityKey: city.Key, Province: city.Province, City: city.Name, Result: result}); err != nil {
				utils.Warn("记录分发访问失败: %v", err)
			}
		}
	}()
	decision, err := store.Check(c.Request.Context(), c.Param("token"), city)
	if err != nil {
		distributionNotice(c, cfg, client, "订阅", "无效的订阅链接", false)
		return
	}
	credentialID = decision.Credential.ID
	if geoErr != nil && decision.Credential.Status == "enabled" {
		decision.Result = "location_unavailable"
	}
	if decision.Result != "allowed" {
		result = decision.Result
		distributionNotice(c, cfg, client, decision.Credential.Name, distributionDecisionMessage(result, city, cfg), result == "expired")
		return
	}
	prepared, ok := distributionPrepared(c, client, decision.Credential.SubscriptionID)
	if !ok {
		c.String(http.StatusServiceUnavailable, "Subscription resources unavailable")
		return
	}
	setDistributionName(&prepared, decision.Credential.Name)
	buffer := renderDistribution(c, prepared)
	if buffer.failed || buffer.status != http.StatusOK || !validDistributionOutput(buffer.body.Bytes(), client) {
		c.String(http.StatusServiceUnavailable, "Subscription generation failed; please retry")
		return
	}
	body, err := distributionBodyPolicy(c, buffer.body.Bytes(), client, cfg)
	if err != nil {
		c.String(http.StatusServiceUnavailable, "Subscription generation failed; please retry")
		return
	}
	decision, err = store.CommitDelivery(c.Request.Context(), c.Param("token"), city, decision.Credential.SubscriptionID)
	if err != nil {
		c.String(http.StatusServiceUnavailable, "Subscription authorization failed; please retry")
		return
	}
	if decision.Result != "allowed" {
		result = decision.Result
		distributionNotice(c, cfg, client, decision.Credential.Name, distributionDecisionMessage(result, city, cfg), result == "expired")
		return
	}
	for key, values := range buffer.headers {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	c.Header("Cache-Control", "no-store, private")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("profile-update-interval", "24")
	if prepared.Subscription.RefreshUsageOnRequest {
		// Traffic follows the resource pool; expiry belongs to this recipient,
		// not the airport. Never recreate a header when usage display is off.
		usage := regexp.MustCompile(`(?:^|;\s*)expire=\d+`).ReplaceAllString(c.Writer.Header().Get("subscription-userinfo"), "")
		if !decision.Credential.Permanent && decision.Credential.ExpiresAt != nil {
			usage += "; expire=" + strconv.FormatInt(decision.Credential.ExpiresAt.Unix(), 10)
		}
		c.Header("subscription-userinfo", strings.Trim(usage, "; "))
	}
	result = "allowed"
	_, _ = c.Writer.Write(body)
}

func validDistributionOutput(body []byte, client string) bool {
	if len(bytes.TrimSpace(body)) == 0 {
		return false
	}
	switch client {
	case "clash", "mihomo", "stash", "egern":
		var cfg struct {
			Proxies []map[string]any `yaml:"proxies"`
		}
		if yaml.Unmarshal(body, &cfg) != nil {
			return false
		}
		return distributionHasProxy(cfg.Proxies)
	case "v2ray", "shadowrocket":
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
		return err == nil && distributionHasLink(decoded)
	case "uri", "v2ray-uri":
		return distributionHasLink(body)
	case "surge", "surfboard", "loon":
		return regexp.MustCompile(`(?im)^[^\r\n=]+?=\s*(?:ss|shadowsocks|vmess|vless|trojan|socks5|http|snell|hysteria2|tuic)\s*,\s*[^,\s]+\s*,\s*\d+`).Match(body)
	case "quanx":
		return regexp.MustCompile(`(?im)^(?:shadowsocks|vmess|vless|trojan|socks5|http)\s*=\s*[^,\s]+:\d+\s*,`).Match(body)
	case "sing-box":
		var cfg struct {
			Outbounds []map[string]any `json:"outbounds"`
		}
		return json.Unmarshal(body, &cfg) == nil && distributionHasProxy(cfg.Outbounds)
	case "json":
		var proxies []map[string]any
		return json.Unmarshal(body, &proxies) == nil && distributionHasProxy(proxies)
	default:
		return false
	}
}

func distributionHasProxy(proxies []map[string]any) bool {
	for _, p := range proxies {
		kind, _ := p["type"].(string)
		server, _ := p["server"].(string)
		if server != "" && kind != "" && kind != "direct" && kind != "reject" && kind != "block" && kind != "selector" && kind != "urltest" {
			return true
		}
	}
	return false
}
func distributionHasLink(body []byte) bool {
	for _, line := range strings.Split(string(body), "\n") {
		p, err := protocol.LinkToProxy(protocol.Urls{Url: strings.TrimSpace(line)}, protocol.OutputConfig{})
		if err == nil && p.Server != "" {
			return true
		}
	}
	return false
}
func distributionDirectRule(body []byte, client, domain string) ([]byte, error) {
	if domain == "" {
		return body, nil
	}
	switch client {
	case "clash", "mihomo":
		var cfg map[string]any
		if err := yaml.Unmarshal(body, &cfg); err != nil {
			return nil, err
		}
		rules, _ := cfg["rules"].([]any)
		cfg["rules"] = append([]any{"DOMAIN," + domain + ",DIRECT"}, rules...)
		return yaml.Marshal(cfg)
	case "surge":
		text := string(body)
		rule := "DOMAIN," + domain + ",DIRECT\n"
		if strings.Contains(text, "[Rule]") {
			text = strings.Replace(text, "[Rule]", "[Rule]\n"+rule, 1)
		} else {
			text += "\n[Rule]\n" + rule
		}
		return []byte(text), nil
	default:
		return body, nil // node-only formats cannot express routing policy
	}
}

var _ gin.ResponseWriter = (*distributionBuffer)(nil)
