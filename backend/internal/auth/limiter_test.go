package auth

import (
	"fmt"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"
	"time"
)

func TestLimiterBoundedCapacityExpiryAndConcurrency(t *testing.T) {
	current := time.Now()
	limit := newLimiter(2, 2, time.Minute)
	limit.now = func() time.Time { return current }
	if ok, _ := limit.allow("first"); !ok {
		t.Fatal("first rejected")
	}
	if ok, _ := limit.allow("first"); !ok {
		t.Fatal("second rejected")
	}
	if ok, retry := limit.allow("first"); ok || retry <= 0 {
		t.Fatal("window limit missing")
	}
	limit.allow("second")
	if ok, _ := limit.allow("third"); ok {
		t.Fatal("capacity limit missing")
	}
	if len(limit.buckets) != 2 {
		t.Fatal("capacity exceeded")
	}
	current = current.Add(time.Minute)
	if ok, _ := limit.allow("third"); !ok {
		t.Fatal("expired buckets not cleared")
	}
	limit = newLimiter(100, 100, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); limit.allow(fmt.Sprint(i)) }(i)
	}
	wg.Wait()
	if len(limit.buckets) > 100 {
		t.Fatal("concurrent calls exceeded capacity")
	}
}

func TestClientIPOnlyTrustsConfiguredImmediatePeer(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	for _, tc := range []struct {
		name, peer, forwarded, expected string
		trust                           []netip.Prefix
	}{
		{"untrusted spoof", "192.0.2.1:1000", "198.51.100.1", "192.0.2.1", trusted},
		{"no trust", "10.1.1.1:1000", "198.51.100.1", "10.1.1.1", nil},
		{"trusted gateway", "10.1.1.1:1000", "198.51.100.1", "198.51.100.1", trusted},
		{"injected left IP", "10.1.1.1:1000", "203.0.113.99, 198.51.100.1", "198.51.100.1", trusted},
		{"trusted chain", "10.1.1.1:1000", "198.51.100.1, 10.2.2.2", "198.51.100.1", trusted},
		{"malformed chain", "10.1.1.1:1000", "not-an-IP", "10.1.1.1", trusted},
		{"mapped IPv4", "[::ffff:192.0.2.1]:1000", "198.51.100.1", "192.0.2.1", trusted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", nil)
			r.RemoteAddr = tc.peer
			r.Header.Set("X-Forwarded-For", tc.forwarded)
			if got := clientIP(r, tc.trust); got != tc.expected {
				t.Fatalf("expected%s got%s", tc.expected, got)
			}
		})
	}
}
