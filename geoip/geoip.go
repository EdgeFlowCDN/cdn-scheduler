package geoip

import (
	"log"
	"math"
	"net"

	"github.com/oschwald/maxminddb-golang"
)

// Location represents a geographic location.
type Location struct {
	Lat float64
	Lon float64
	ISP string
}

// Locator resolves IP addresses to geographic locations.
type Locator interface {
	Lookup(ip string) *Location
}

// StaticLocator uses a static map for IP lookups (for testing/development).
type StaticLocator struct {
	entries  map[string]*Location
	fallback *Location
}

// NewStaticLocator creates a locator with predefined IP mappings.
func NewStaticLocator() *StaticLocator {
	return &StaticLocator{
		entries: map[string]*Location{
			// Example entries for testing
			"1.0.0.0": {Lat: 39.9, Lon: 116.4, ISP: "telecom"}, // Beijing
			"2.0.0.0": {Lat: 31.2, Lon: 121.5, ISP: "unicom"},  // Shanghai
			"3.0.0.0": {Lat: 23.1, Lon: 113.3, ISP: "mobile"},  // Guangzhou
		},
		fallback: &Location{Lat: 39.9, Lon: 116.4, ISP: "unknown"},
	}
}

func (s *StaticLocator) Lookup(ip string) *Location {
	if loc, ok := s.entries[ip]; ok {
		return loc
	}
	return s.fallback
}

// AddEntry adds an IP -> Location mapping.
func (s *StaticLocator) AddEntry(ip string, loc *Location) {
	s.entries[ip] = loc
}

// maxmindRecord represents the data structure stored in MaxMind .mmdb files.
type maxmindRecord struct {
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Location struct {
		Latitude  float64 `maxminddb:"latitude"`
		Longitude float64 `maxminddb:"longitude"`
	} `maxminddb:"location"`
	Traits struct {
		ISP          string `maxminddb:"isp"`
		Organization string `maxminddb:"organization"`
	} `maxminddb:"traits"`
}

// MaxMindLocator uses MaxMind GeoLite2/GeoIP2 database.
type MaxMindLocator struct {
	db       *maxminddb.Reader
	fallback *Location
}

// NewMaxMindLocator creates a locator using a MaxMind .mmdb file.
// Falls back to StaticLocator if the file cannot be opened.
func NewMaxMindLocator(dbPath string) Locator {
	reader, err := maxminddb.Open(dbPath)
	if err != nil {
		log.Printf("warning: failed to open MaxMind database %s: %v, falling back to static locator", dbPath, err)
		return NewStaticLocator()
	}
	log.Printf("loaded MaxMind GeoIP database: %s", dbPath)
	return &MaxMindLocator{
		db:       reader,
		fallback: &Location{Lat: 39.9, Lon: 116.4, ISP: "unknown"},
	}
}

// Lookup resolves an IP address to a geographic location using the MaxMind database.
func (m *MaxMindLocator) Lookup(ipStr string) *Location {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return m.fallback
	}

	var record maxmindRecord
	err := m.db.Lookup(ip, &record)
	if err != nil {
		return m.fallback
	}

	// If no location data found, return fallback
	if record.Location.Latitude == 0 && record.Location.Longitude == 0 {
		return m.fallback
	}

	isp := record.Traits.ISP
	if isp == "" {
		isp = record.Traits.Organization
	}
	if isp == "" {
		isp = "unknown"
	}

	return &Location{
		Lat: record.Location.Latitude,
		Lon: record.Location.Longitude,
		ISP: isp,
	}
}

// Close releases the MaxMind database resources.
func (m *MaxMindLocator) Close() {
	if m.db != nil {
		m.db.Close()
	}
}

// Distance calculates the great-circle distance between two points in km.
func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371 // Earth radius in km

	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// IsPrivateIP checks if an IP is in a private range.
func IsPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	privateRanges := []struct {
		start net.IP
		end   net.IP
	}{
		{net.ParseIP("10.0.0.0"), net.ParseIP("10.255.255.255")},
		{net.ParseIP("172.16.0.0"), net.ParseIP("172.31.255.255")},
		{net.ParseIP("192.168.0.0"), net.ParseIP("192.168.255.255")},
		{net.ParseIP("127.0.0.0"), net.ParseIP("127.255.255.255")},
	}
	for _, r := range privateRanges {
		if bytesCompare(ip.To4(), r.start.To4()) >= 0 && bytesCompare(ip.To4(), r.end.To4()) <= 0 {
			return true
		}
	}
	return false
}

func bytesCompare(a, b net.IP) int {
	if a == nil || b == nil {
		return 0
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
