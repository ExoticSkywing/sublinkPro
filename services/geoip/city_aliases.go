package geoip

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/oschwald/geoip2-golang/v2"
	"github.com/oschwald/maxminddb-golang/v2"
	"golang.org/x/text/unicode/norm"
)

// Build aliases from the installed database, not a second hand-maintained city
// list. Existing CN:<GeoNames ID> grants remain valid across providers.
type cityIndex struct {
	cities    map[string]CityLocation
	provinces map[string]string
}

func cityName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, suffix := range []string{" province", " city", "壮族自治区", "回族自治区", "维吾尔自治区", "自治区", "省", "市"} {
		value = strings.TrimSuffix(value, suffix)
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, norm.NFD.String(value))
}

func (idx *cityIndex) add(record *geoip2.City) {
	if record.Country.ISOCode != "CN" || record.City.GeoNameID == 0 || len(record.Subdivisions) == 0 {
		return
	}
	province := record.Subdivisions[0]
	canonical := fmt.Sprint(province.GeoNameID)
	if canonical == "0" {
		return
	}
	out := locationFromRecord(record)
	for _, name := range []string{province.Names.SimplifiedChinese, province.Names.English, province.ISOCode} {
		p := cityName(name)
		if p == "" {
			continue
		}
		if old, exists := idx.provinces[p]; exists && old != canonical {
			idx.provinces[p] = ""
		} else if !exists {
			idx.provinces[p] = canonical
		}
		for _, alias := range []string{record.City.Names.SimplifiedChinese, record.City.Names.English} {
			c := cityName(alias)
			if c == "" {
				continue
			}
			key := p + ":" + c
			if old, exists := idx.cities[key]; exists && old.Key != out.Key {
				idx.cities[key] = CityLocation{} // Ambiguous names must never grant a city.
			} else if !exists {
				idx.cities[key] = out
			}
		}
	}
}

func loadCityIndex(path string) (*cityIndex, error) {
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	idx := &cityIndex{cities: map[string]CityLocation{}, provinces: map[string]string{}}
	seen := map[uint]bool{}
	for result := range db.Networks() {
		var country string
		if err := result.DecodePath(&country, "country", "iso_code"); err != nil {
			return nil, err
		}
		if country != "CN" {
			continue
		}
		var record geoip2.City
		if err := result.Decode(&record); err != nil {
			return nil, err
		}
		if seen[record.City.GeoNameID] {
			continue
		}
		seen[record.City.GeoNameID] = true
		idx.add(&record)
	}
	return idx, nil
}

func normalizeCity(location CityLocation) CityLocation {
	if location.Country != "CN" || location.Key != "" {
		return location
	}
	mu.RLock()
	defer mu.RUnlock()
	if cityAliases != nil {
		if known := cityAliases.cities[cityName(location.Province)+":"+cityName(location.City)]; known.Key != "" {
			known.Source = location.Source
			return known
		}
	}
	return location
}

func locationsConflict(a, b CityLocation) bool {
	if a.Country != "" && b.Country != "" && a.Country != b.Country {
		return true
	}
	if a.Key != "" && b.Key != "" && a.Key != b.Key {
		return true
	}
	// If an earlier provider gave an unmappable city, a different city name is
	// not evidence that it was merely missing data. Require review, not a guess.
	if a.Country == "CN" && b.Country == "CN" && a.City != "" && b.City != "" &&
		(a.Key == "" || b.Key == "") && cityName(a.City) != cityName(b.City) {
		return true
	}
	mu.RLock()
	defer mu.RUnlock()
	if cityAliases == nil {
		return false
	}
	p, q := cityAliases.provinces[cityName(a.Province)], cityAliases.provinces[cityName(b.Province)]
	return p != "" && q != "" && p != q
}
