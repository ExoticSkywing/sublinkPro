package geoip

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"sublink/config"
	"sublink/utils"

	"github.com/lionsoul2014/ip2region/binding/golang/service"
	"golang.org/x/sync/singleflight"
)

var errCityUnavailable = errors.New("city location unavailable")

type cityLookup func(context.Context, string) (CityLocation, error)
type cachedCity struct {
	location CityLocation
	err      error
	expires  time.Time
}
type cityResolver struct {
	primary, offline, remote cityLookup
	normalize                func(CityLocation) CityLocation
	conflict                 func(CityLocation, CityLocation) bool
	now                      func() time.Time
	mu                       sync.Mutex
	cache                    map[string]cachedCity
	flights                  singleflight.Group
}

func newCityResolver(primary, offline, remote cityLookup) *cityResolver {
	return &cityResolver{primary: primary, offline: offline, remote: remote, normalize: normalizeCity,
		conflict: locationsConflict, now: time.Now, cache: make(map[string]cachedCity)}
}

func (r *cityResolver) cached(key string) (cachedCity, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.cache[key]
	return v, ok && r.now().Before(v.expires)
}

func (r *cityResolver) lookup(ctx context.Context, ipStr string) (CityLocation, error) {
	ip, err := netip.ParseAddr(ipStr)
	if err != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.Zone() != "" {
		return CityLocation{}, errCityUnavailable
	}
	ip = ip.Unmap()
	if netip.MustParsePrefix("100.64.0.0/10").Contains(ip) {
		return CityLocation{}, errCityUnavailable
	}
	if err := ctx.Err(); err != nil {
		return CityLocation{}, err
	}
	ipStr = ip.String()
	key := fmt.Sprintf("%d:%s", cityGeneration.Load(), ipStr)
	if hit, ok := r.cached(key); ok {
		return hit.location, hit.err
	}
	result := r.flights.DoChan(key, func() (any, error) {
		if hit, ok := r.cached(key); ok {
			return hit.location, hit.err
		}
		// One cancelled caller must not cancel a shared lookup for other clients.
		queryCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		location, err := r.resolve(queryCtx, ipStr)
		ttl := 24 * time.Hour
		if err != nil {
			ttl = time.Minute
		}
		r.mu.Lock()
		if len(r.cache) >= 4096 {
			var oldest string
			var deadline time.Time
			for k, v := range r.cache {
				if oldest == "" || v.expires.Before(deadline) {
					oldest, deadline = k, v.expires
				}
			}
			delete(r.cache, oldest)
		}
		r.cache[key] = cachedCity{location: location, err: err, expires: r.now().Add(ttl)}
		r.mu.Unlock()
		return location, err
	})
	select {
	case <-ctx.Done():
		return CityLocation{}, ctx.Err()
	case answer := <-result:
		location, ok := answer.Val.(CityLocation)
		if !ok {
			return CityLocation{}, errCityUnavailable
		}
		return location, answer.Err
	}
}

func (r *cityResolver) resolve(ctx context.Context, ip string) (CityLocation, error) {
	var previous []CityLocation
	for _, lookup := range []cityLookup{r.primary, r.offline, r.remote} {
		if lookup == nil {
			continue
		}
		location, err := lookup(ctx, ip)
		if err != nil {
			continue
		}
		location = r.normalize(location)
		for _, prior := range previous {
			if r.conflict(prior, location) {
				return CityLocation{}, errCityUnavailable
			}
		}
		// A known foreign country is terminal, not a reason to try another source.
		if location.Country != "" && location.Country != "CN" {
			return location, nil
		}
		if location.Country == "CN" && location.Key != "" && location.City != "" {
			return location, nil
		}
		previous = append(previous, location)
	}
	if len(previous) > 0 {
		return previous[0], errCityUnavailable
	}
	return CityLocation{}, errCityUnavailable
}

var defaultCityResolver = sync.OnceValue(func() *cityResolver {
	cfg := config.GetDistributionGeoIPSettings()
	var v4, v6 *service.Config
	var err error
	v4, err = service.NewV4Config(service.BufferCache, cfg.IPv4Path, 1)
	if err != nil {
		utils.Warn("分发 IPv4 离线城市兜底未加载")
	}
	v6, err = service.NewV6Config(service.BufferCache, cfg.IPv6Path, 1)
	if err != nil {
		utils.Warn("分发 IPv6 离线城市兜底未加载")
	}
	local, err := service.NewIp2Region(v4, v6)
	var offline cityLookup
	if err == nil {
		offline = func(_ context.Context, ip string) (CityLocation, error) {
			region, err := local.Search(ip)
			if err != nil {
				return CityLocation{}, errCityUnavailable
			}
			return parseOfflineCity(region), nil
		}
	}
	api := newIPDataClient(cfg.APIKey, cfg.DailyLimit)
	return newCityResolver(func(_ context.Context, ip string) (CityLocation, error) { return getMaxMindCity(ip) }, offline, api.lookup)
})

func GetCityLocationContext(ctx context.Context, ip string) (CityLocation, error) {
	return defaultCityResolver().lookup(ctx, ip)
}
