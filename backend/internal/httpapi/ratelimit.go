package httpapi

import (
	"net/netip"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ipLimiter is an in-memory token bucket per client: perMin requests a minute, bursting up to perMin. An IPv6
// client is keyed by its /64, the smallest block ISPs assign to one subscriber; keying by the full address would give
// a single subscriber 2^64 buckets. It holds per process; a second backend replica would double the limit.
type ipLimiter struct {
	every  rate.Limit
	burst  int
	refill time.Duration // time for an empty bucket to fill again
	now    func() time.Time

	mu        sync.Mutex
	buckets   map[netip.Addr]*bucket
	lastSweep time.Time
}

type bucket struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

func newIPLimiter(perMin int, now func() time.Time) *ipLimiter {
	return &ipLimiter{
		every:     rate.Every(time.Minute / time.Duration(perMin)),
		burst:     perMin,
		refill:    time.Minute,
		now:       now,
		buckets:   map[netip.Addr]*bucket{},
		lastSweep: now(),
	}
}

// wait takes a token for addr and returns 0, or, when the bucket is empty, how long until one is available.
func (l *ipLimiter) wait(addr netip.Addr) time.Duration {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	key := limitKey(addr)
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{lim: rate.NewLimiter(l.every, l.burst)}
		l.buckets[key] = b
	}
	b.lastSeen = now
	res := b.lim.ReserveN(now, 1)
	if d := res.DelayFrom(now); d > 0 {
		res.CancelAt(now)
		return d
	}
	return 0
}

// limitKey is addr itself for IPv4 and the first address of addr's /64 for IPv6.
func limitKey(addr netip.Addr) netip.Addr {
	if addr.Is6() {
		return netip.PrefixFrom(addr, 64).Masked().Addr()
	}
	return addr
}

// sweep drops buckets idle for longer than a refill: they are full again, the same as a new one, so dropping them
// changes no outcome. It runs at most once per refill, inside wait, so no goroutine outlives the handler.
func (l *ipLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < l.refill {
		return
	}
	l.lastSweep = now
	for addr, b := range l.buckets {
		if now.Sub(b.lastSeen) >= l.refill {
			delete(l.buckets, addr)
		}
	}
}
