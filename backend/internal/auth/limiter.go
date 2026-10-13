package auth

import (
	"crypto/sha256"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

type rateBucket struct {
	count   int
	expires time.Time
}

// Each limiter is bounded. When full, new identities are rejected until existing
// windows expire; attacker-controlled identifiers cannot grow memory indefinitely.
type limiter struct {
	mu              sync.Mutex
	buckets         map[[32]byte]rateBucket
	limit, capacity int
	window          time.Duration
	now             func() time.Time
	nextSweep       time.Time
}

func newLimiter(limit, capacity int, window time.Duration) *limiter {
	return &limiter{buckets: make(map[[32]byte]rateBucket), limit: limit, capacity: capacity, window: window, now: time.Now}
}
func (l *limiter) allow(identity string) (bool, time.Duration) {
	key := sha256.Sum256([]byte(identity))
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !now.Before(l.nextSweep) {
		for key, bucket := range l.buckets {
			if !now.Before(bucket.expires) {
				delete(l.buckets, key)
			}
		}
		l.nextSweep = now.Add(time.Minute)
	}
	bucket, exists := l.buckets[key]
	if !exists || !now.Before(bucket.expires) {
		if !exists && len(l.buckets) >= l.capacity {
			return false, time.Minute
		}
		bucket = rateBucket{expires: now.Add(l.window)}
	}
	if bucket.count >= l.limit {
		return false, bucket.expires.Sub(now)
	}
	bucket.count++
	l.buckets[key] = bucket
	return true, 0
}

func clientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown-peer"
	}
	peer = peer.Unmap()
	isTrusted := func(addr netip.Addr) bool {
		for _, prefix := range trusted {
			if prefix.Contains(addr) {
				return true
			}
		}
		return false
	}
	if !isTrusted(peer) {
		return peer.String()
	}
	// Only a bounded chain from an explicitly trusted peer is examined.
	values := r.Header.Values("X-Forwarded-For")
	if len(values) != 1 || len(values[0]) > 2048 {
		return peer.String()
	}
	chain := strings.Split(values[0], ",")
	if len(chain) > 32 {
		return peer.String()
	}
	for i := len(chain) - 1; i >= 0; i-- {
		address, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
		if err != nil {
			return peer.String()
		}
		address = address.Unmap()
		if !isTrusted(address) {
			return address.String()
		}
		peer = address
	}
	return peer.String()
}
