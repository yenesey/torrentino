package web

import (
	"context"
	"sync"
	"time"

	"torrentino/core"
)

// downloadsCache coalesces /downloads polls from several tabs. A snapshot is
// reused for two seconds, and concurrent misses wait for one fetch instead of
// hitting Transmission and the disk once per tab.
type downloadsCache struct {
	mu      sync.Mutex
	fetchMu sync.Mutex
	items   []*core.DownloadItem
	stats   core.DiskStats
	updated time.Time
	err     error
	ttl     time.Duration
	now     func() time.Time
}

func newDownloadsCache(ttl time.Duration) *downloadsCache {
	return &downloadsCache{
		items: make([]*core.DownloadItem, 0),
		ttl:   ttl,
		now:   time.Now,
	}
}

// Get returns the cached snapshot or fetches a new one. On a fetch error the
// previous items are returned together with the error, so the UI can keep the
// old table and show a warning.
func (c *downloadsCache) Get(ctx context.Context, service *core.Service) ([]*core.DownloadItem, core.DiskStats, time.Time, error) {
	c.mu.Lock()
	if c.freshLocked() {
		items, stats, updated, err := c.items, c.stats, c.updated, c.err
		c.mu.Unlock()
		return items, stats, updated, err
	}
	c.mu.Unlock()

	c.fetchMu.Lock()
	defer c.fetchMu.Unlock()

	c.mu.Lock()
	if c.freshLocked() {
		items, stats, updated, err := c.items, c.stats, c.updated, c.err
		c.mu.Unlock()
		return items, stats, updated, err
	}
	c.mu.Unlock()

	items, stats, err := service.Downloads(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		c.items = items
		c.stats = stats
		c.updated = c.now()
		c.err = nil
		return c.items, c.stats, c.updated, nil
	}
	c.err = err
	c.updated = c.now()
	// Keep the previous successful snapshot; an empty initial failure is
	// returned as an empty list. The attempt is cached for the TTL too, so a
	// broken Transmission is not hammered once per tab.
	return c.items, c.stats, c.updated, err
}

func (c *downloadsCache) freshLocked() bool {
	if c.updated.IsZero() {
		return false
	}
	return c.now().Sub(c.updated) < c.ttl
}

// Invalidate forces the next request to refetch. It is used after a mutation
// so a delete is not hidden by a stale cache.
func (c *downloadsCache) Invalidate() {
	// Wait for an in-flight fetch before clearing: otherwise that fetch could
	// finish after the invalidation and store a snapshot that still contains
	// the item we just deleted.
	c.fetchMu.Lock()
	defer c.fetchMu.Unlock()
	c.mu.Lock()
	c.updated = time.Time{}
	c.err = nil
	c.mu.Unlock()
}
