package geoip

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/oschwald/geoip2-golang/v2"
)

// CityLocation uses GeoNames IDs for access rules; translated display names are
// not identifiers. A country-only database deliberately cannot authorize a city.
type CityLocation struct {
	Key      string
	Country  string
	Province string
	City     string
	Source   string
}

func GetCityLocation(ipStr string) (CityLocation, error) {
	return GetCityLocationContext(context.Background(), ipStr)
}

func getMaxMindCity(ipStr string) (CityLocation, error) {
	mu.RLock()
	defer mu.RUnlock()
	var out CityLocation
	if !available || geoIP == nil {
		return out, fmt.Errorf("GeoIP database unavailable")
	}
	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return out, err
	}
	record, err := geoIP.City(ip.Unmap())
	if err != nil {
		return out, err
	}
	return locationFromRecord(record), nil
}

func locationFromRecord(record *geoip2.City) CityLocation {
	out := CityLocation{Source: "geolite2"}
	out.Country = record.Country.ISOCode
	out.City = record.City.Names.SimplifiedChinese
	if out.City == "" {
		out.City = record.City.Names.English
	}
	if len(record.Subdivisions) > 0 {
		out.Province = record.Subdivisions[0].Names.SimplifiedChinese
		if out.Province == "" {
			out.Province = record.Subdivisions[0].Names.English
		}
	}
	if record.City.GeoNameID > 0 && out.Country != "" && out.City != "" {
		out.Key = fmt.Sprintf("%s:%d", out.Country, record.City.GeoNameID)
	}
	return out
}
