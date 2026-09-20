package geoip

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oschwald/geoip2-golang/v2"
)

func installTestCities(t *testing.T) {
	t.Helper()
	idx := &cityIndex{cities: map[string]CityLocation{}, provinces: map[string]string{}}
	for _, data := range []string{
		`{"country":{"iso_code":"CN"},"city":{"geoname_id":2036389,"names":{"en":"Jixi","zh-CN":"鸡西市"}},"subdivisions":[{"geoname_id":2036965,"iso_code":"HL","names":{"en":"Heilongjiang","zh-CN":"黑龙江"}}]}`,
		`{"country":{"iso_code":"CN"},"city":{"geoname_id":1791247,"names":{"en":"Wuhan","zh-CN":"武汉"}},"subdivisions":[{"geoname_id":1806949,"iso_code":"HB","names":{"en":"Hubei","zh-CN":"湖北"}}]}`,
	} {
		var record geoip2.City
		if err := json.Unmarshal([]byte(data), &record); err != nil {
			t.Fatal(err)
		}
		idx.add(&record)
	}
	mu.Lock()
	old := cityAliases
	cityAliases = idx
	mu.Unlock()
	t.Cleanup(func() { mu.Lock(); cityAliases = old; mu.Unlock() })
}

func TestCityAliasesPreserveExistingGrants(t *testing.T) {
	installTestCities(t)
	for _, city := range []CityLocation{
		{Country: "CN", Province: "黑龙江省", City: "鸡西市", Source: "ip2region"},
		{Country: "CN", Province: "Heilongjiang", City: "Jixi", Source: "ipdata"},
		{Country: "CN", Province: "HL", City: "Jixi City", Source: "ipdata"},
	} {
		got := normalizeCity(city)
		if got.Key != "CN:2036389" || got.Source != city.Source {
			t.Fatalf("alias not mapped: %+v", got)
		}
	}
	for _, city := range []CityLocation{
		{Country: "CN", Province: "Hubei", City: "Jixi"},
		{Country: "CN", City: "Jixi"},
		{Country: "CN", Province: "Heilongjiang", City: "Unknown"},
	} {
		if normalizeCity(city).Key != "" {
			t.Fatal("unknown/ambiguous city granted")
		}
	}
	var duplicate geoip2.City
	if err := json.Unmarshal([]byte(`{"country":{"iso_code":"CN"},"city":{"geoname_id":123,"names":{"en":"Jixi"}},"subdivisions":[{"geoname_id":2036965,"iso_code":"HL","names":{"en":"Heilongjiang"}}]}`), &duplicate); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	cityAliases.add(&duplicate)
	mu.Unlock()
	if normalizeCity(CityLocation{Country: "CN", Province: "HL", City: "Jixi"}).Key != "" {
		t.Fatal("ambiguous alias granted")
	}
}

func TestCityFallbackOrderAndDenials(t *testing.T) {
	installTestCities(t)
	complete := CityLocation{Country: "CN", Key: "CN:2036389", Province: "黑龙江", City: "鸡西市"}
	for _, tc := range []struct {
		name                     string
		primary, offline, remote CityLocation
		wantKey, wantCountry     string
		wantErr                  bool
		wantCalls                int
	}{
		{"primary", complete, CityLocation{}, CityLocation{}, complete.Key, "CN", false, 1},
		{"offline", CityLocation{Country: "CN"}, CityLocation{Country: "CN", Province: "黑龙江省", City: "鸡西市"}, CityLocation{}, complete.Key, "CN", false, 2},
		{"remote", CityLocation{Country: "CN"}, CityLocation{}, CityLocation{Country: "CN", Province: "Heilongjiang", City: "Jixi"}, complete.Key, "CN", false, 3},
		{"foreign primary", CityLocation{Country: "JP"}, complete, complete, "", "JP", false, 1},
		{"foreign offline", CityLocation{}, CityLocation{Country: "HK"}, complete, "", "HK", false, 2},
		{"country conflict", CityLocation{Country: "CN"}, CityLocation{Country: "JP"}, complete, "", "", true, 2},
		{"province conflict", CityLocation{Country: "CN", Province: "湖北"}, complete, complete, "", "", true, 2},
		{"unmapped city conflict", CityLocation{Country: "CN", Province: "黑龙江", City: "Unknown"}, complete, complete, "", "", true, 2},
		{"no city", CityLocation{Country: "CN"}, CityLocation{}, CityLocation{Country: "CN"}, "", "CN", true, 3},
		{"unknown alias", CityLocation{Country: "CN"}, CityLocation{}, CityLocation{Country: "CN", Province: "HL", City: "Unknown"}, "", "CN", true, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			lookup := func(v CityLocation) cityLookup {
				return func(context.Context, string) (CityLocation, error) { calls++; return v, nil }
			}
			r := newCityResolver(lookup(tc.primary), lookup(tc.offline), lookup(tc.remote))
			got, err := r.lookup(context.Background(), "1.2.3.4")
			if (err != nil) != tc.wantErr || got.Key != tc.wantKey || got.Country != tc.wantCountry || calls != tc.wantCalls {
				t.Fatalf("got %+v %v calls=%d", got, err, calls)
			}
			_, _ = r.lookup(context.Background(), "::ffff:1.2.3.4")
			if calls != tc.wantCalls {
				t.Fatal("mapped IP missed cache")
			}
		})
	}
}

func TestCityCacheExpiryAndGeneration(t *testing.T) {
	now := time.Now()
	calls := 0
	r := newCityResolver(func(context.Context, string) (CityLocation, error) {
		calls++
		return CityLocation{Country: "CN", Key: "CN:1", City: "City"}, nil
	}, nil, nil)
	r.now = func() time.Time { return now }
	_, _ = r.lookup(context.Background(), "1.2.3.4")
	now = now.Add(23 * time.Hour)
	_, _ = r.lookup(context.Background(), "1.2.3.4")
	if calls != 1 {
		t.Fatal("cache expired early")
	}
	now = now.Add(2 * time.Hour)
	_, _ = r.lookup(context.Background(), "1.2.3.4")
	if calls != 2 {
		t.Fatal("stale cache used")
	}
	cityGeneration.Add(1)
	_, _ = r.lookup(context.Background(), "1.2.3.4")
	if calls != 3 {
		t.Fatal("database reload did not invalidate cache")
	}
	// Seed to capacity and verify the next insertion evicts one entry.
	r.cache = map[string]cachedCity{}
	for i := 0; i < 4096; i++ {
		r.cache[fmt.Sprint(i)] = cachedCity{expires: now.Add(time.Hour)}
	}
	_, _ = r.lookup(context.Background(), "1.2.3.5")
	if len(r.cache) != 4096 {
		t.Fatal("cache grew past its bound")
	}
	failedCalls := 0
	r = newCityResolver(func(context.Context, string) (CityLocation, error) {
		failedCalls++
		return CityLocation{}, errCityUnavailable
	}, nil, nil)
	r.now = func() time.Time { return now }
	_, _ = r.lookup(context.Background(), "1.2.3.4")
	_, _ = r.lookup(context.Background(), "1.2.3.4")
	if failedCalls != 1 {
		t.Fatal("negative cache missed")
	}
	now = now.Add(61 * time.Second)
	_, _ = r.lookup(context.Background(), "1.2.3.4")
	if failedCalls != 2 {
		t.Fatal("negative cache did not expire")
	}
}

func TestCitySingleflightAndCancellation(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	r := newCityResolver(func(context.Context, string) (CityLocation, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return CityLocation{Country: "JP"}, nil
	}, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := r.lookup(ctx, "1.2.3.4"); done <- err }()
	<-started
	cancel()
	if !errors.Is(<-done, context.Canceled) {
		t.Fatal("caller cancellation ignored")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := r.lookup(context.Background(), "1.2.3.4")
			if err != nil || got.Country != "JP" {
				t.Error("shared lookup cancelled")
			}
		}()
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("concurrent IP lookups not merged")
	}
}

func TestCityRejectsPrivateInputsWithoutLookups(t *testing.T) {
	r := newCityResolver(func(context.Context, string) (CityLocation, error) {
		t.Fatal("invalid IP reached provider")
		return CityLocation{}, nil
	}, nil, nil)
	for _, ip := range []string{"invalid", "127.0.0.1", "::1", "10.0.0.1", "::ffff:10.0.0.1", "100.64.0.1", "169.254.1.1", "fe80::1", "ff02::1", "2001:4860::1%eth0"} {
		if _, err := r.lookup(context.Background(), ip); err == nil {
			t.Fatalf("accepted %s", ip)
		}
	}
}

type cityRoundTrip func(*http.Request) (*http.Response, error)

func (f cityRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestIPDataResponseValidationAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantErr    bool
	}{
		{"valid", `{"ip":"1.2.3.4","country_code":"CN","region":"Heilongjiang","city":"Jixi"}`, 200, false},
		{"wrong IP", `{"ip":"8.8.8.8","country_code":"CN","region":"Heilongjiang","city":"Jixi"}`, 200, true},
		{"bad country", `{"ip":"1.2.3.4","country_code":"China"}`, 200, true},
		{"invalid JSON", "invalid", 200, true},
		{"oversized", strings.Repeat("x", 16385), 200, true},
		{"quota", "secret-key", 429, true},
		{"key rejected", "secret-key", 403, true},
		{"redirect", "", 302, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newIPDataClient("secret-key", 10)
			p.client.Transport = cityRoundTrip(func(r *http.Request) (*http.Response, error) {
				q := r.URL.Query()
				if r.URL.Scheme != "https" || r.URL.Host != "api.ipdata.co" || r.URL.Path != "/1.2.3.4" || q.Get("api-key") != "secret-key" || len(q) != 2 {
					t.Fatal("unexpected outbound request")
				}
				if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Referer") != "" {
					t.Fatal("user credentials forwarded")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})
			_, err := p.lookup(context.Background(), "1.2.3.4")
			if (err != nil) != tc.wantErr {
				t.Fatalf("error %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "secret-key") {
				t.Fatal("secret leaked")
			}
			if tc.status == 429 || tc.status == 403 {
				if p.reserve() {
					t.Fatal("cooldown ignored")
				}
			}
		})
	}
}

func TestIPDataLimitTimeoutAndRedirect(t *testing.T) {
	p := newIPDataClient("key", 1)
	if !p.reserve() || p.reserve() {
		t.Fatal("daily limit ignored")
	}
	if newIPDataClient("", 1).reserve() || newIPDataClient("key", 0).reserve() {
		t.Fatal("disabled API used")
	}
	p.block(time.Hour)
	until := p.blockedUntil
	p.block(time.Minute)
	if p.blockedUntil.Before(until) {
		t.Fatal("concurrent shorter failure weakened cooldown")
	}
	if p.client.Timeout != 2*time.Second || p.client.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("timeout/redirect protection missing")
	}
	p = newIPDataClient("key", 5)
	p.client.Transport = cityRoundTrip(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, errors.New("request URL contained key")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := p.lookup(ctx, "1.2.3.4")
	if err != errCityUnavailable {
		t.Fatal("raw transport error escaped")
	}
}

func TestOfflineRegionFormats(t *testing.T) {
	got := parseOfflineCity("中国|黑龙江省|鸡西市|联通|CN")
	if got.Country != "CN" || got.City != "鸡西市" {
		t.Fatalf("%+v", got)
	}
	for _, value := range []string{"中国|0|黑龙江|鸡西|联通", "中国|黑龙江|鸡西", "Reserved|Reserved|Reserved|0|0"} {
		if parseOfflineCity(value).Country != "" {
			t.Fatal("legacy or reserved format accepted")
		}
	}
}
