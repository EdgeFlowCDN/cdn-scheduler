package geoip

import (
	"math"
	"testing"
)

func TestDistance(t *testing.T) {
	// Beijing to Shanghai ≈ 1068 km
	d := Distance(39.9, 116.4, 31.2, 121.5)
	if d < 1000 || d > 1200 {
		t.Errorf("Beijing-Shanghai distance = %.0f km, expected ~1068", d)
	}

	// Same point
	d = Distance(39.9, 116.4, 39.9, 116.4)
	if d != 0 {
		t.Errorf("same point distance = %f, expected 0", d)
	}
}

func TestStaticLocator(t *testing.T) {
	loc := NewStaticLocator()

	// Known IP
	l := loc.Lookup("1.0.0.0")
	if l == nil {
		t.Fatal("expected location for known IP")
	}
	if l.ISP != "telecom" {
		t.Errorf("ISP = %q, want telecom", l.ISP)
	}

	// Unknown IP should return fallback
	l = loc.Lookup("9.9.9.9")
	if l == nil {
		t.Fatal("expected fallback location")
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"127.0.0.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}

	for _, tt := range tests {
		got := IsPrivateIP(tt.ip)
		if got != tt.private {
			t.Errorf("IsPrivateIP(%q) = %v, want %v", tt.ip, got, tt.private)
		}
	}
}

func TestDistanceSymmetry(t *testing.T) {
	d1 := Distance(39.9, 116.4, 31.2, 121.5)
	d2 := Distance(31.2, 121.5, 39.9, 116.4)
	if math.Abs(d1-d2) > 0.01 {
		t.Errorf("distance should be symmetric: %f vs %f", d1, d2)
	}
}
