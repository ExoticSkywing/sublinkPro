package geoip

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"sublink/utils"
)

func parseOfflineCity(region string) CityLocation {
	// Only the documented v3 Country|Province|City|ISP|ISO format is accepted.
	parts := strings.Split(region, "|")
	if len(parts) != 5 {
		return CityLocation{}
	}
	out := CityLocation{Country: parts[4], Source: "ip2region"}
	if len(out.Country) != 2 || out.Country != strings.ToUpper(out.Country) {
		return CityLocation{}
	}
	if parts[1] != "0" {
		out.Province = parts[1]
	}
	if parts[2] != "0" {
		out.City = parts[2]
	}
	return out
}

type ipdataClient struct {
	key                  string
	client               *http.Client
	limit                int
	limiter              *rate.Limiter
	mu                   sync.Mutex
	window, blockedUntil time.Time
	used                 int
	now                  func() time.Time
}

func newIPDataClient(key string, limit int) *ipdataClient {
	// Direct HTTPS, never the subscriber's nodes or environment proxy.
	transport := &http.Transport{MaxIdleConns: 8, MaxIdleConnsPerHost: 4, MaxConnsPerHost: 4,
		IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 2 * time.Second, ForceAttemptHTTP2: true}
	return &ipdataClient{key: key, limit: limit, now: time.Now, limiter: rate.NewLimiter(1, 5),
		client: &http.Client{Transport: transport, Timeout: 2 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (p *ipdataClient) reserve() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	if p.key == "" || p.limit <= 0 || now.Before(p.blockedUntil) {
		return false
	}
	if p.window.IsZero() || !now.Before(p.window.Add(24*time.Hour)) {
		p.window, p.used = now, 0
	}
	if p.used >= p.limit || !p.limiter.AllowN(now, 1) {
		return false
	}
	p.used++
	return true
}

func (p *ipdataClient) block(duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	until := p.now().Add(duration)
	if !until.After(p.blockedUntil) {
		return
	}
	p.blockedUntil = until
	// Never log request URLs, raw errors or response bodies: they may contain the key.
	utils.Warn("ipdata 城市兜底暂不可用，暂停外部查询 %s；本地定位不受影响", duration)
}

func (p *ipdataClient) lookup(ctx context.Context, ip string) (CityLocation, error) {
	address, err := netip.ParseAddr(ip)
	if err != nil || !address.IsGlobalUnicast() || address.IsPrivate() || address.Zone() != "" {
		return CityLocation{}, errCityUnavailable
	}
	if !p.reserve() {
		return CityLocation{}, errCityUnavailable
	}
	query := url.Values{"api-key": {p.key}, "fields": {"ip,country_code,region,region_code,city"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipdata.co/"+address.Unmap().String()+"?"+query.Encode(), nil)
	if err != nil {
		return CityLocation{}, errCityUnavailable
	}
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		p.block(time.Minute)
		return CityLocation{}, errCityUnavailable
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		delay := time.Minute
		if response.StatusCode == http.StatusTooManyRequests {
			delay = 15 * time.Minute
		}
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			delay = time.Hour
		}
		p.block(delay)
		return CityLocation{}, errCityUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 16*1024+1))
	if err != nil || len(body) > 16*1024 {
		return CityLocation{}, errCityUnavailable
	}
	var record struct {
		IP           string `json:"ip"`
		Country      string `json:"country_code"`
		Province     string `json:"region"`
		ProvinceCode string `json:"region_code"`
		City         string `json:"city"`
	}
	if json.Unmarshal(body, &record) != nil {
		return CityLocation{}, errCityUnavailable
	}
	returned, err := netip.ParseAddr(record.IP)
	if err != nil || returned.Unmap() != address.Unmap() || len(record.Country) != 2 || record.Country != strings.ToUpper(record.Country) || len(record.Province) > 128 || len(record.City) > 128 || len(record.ProvinceCode) > 16 {
		return CityLocation{}, errCityUnavailable
	}
	out := CityLocation{Country: record.Country, Province: record.Province, City: record.City, Source: "ipdata"}
	if out.Province == "" {
		out.Province = record.ProvinceCode
	}
	return out, nil
}
