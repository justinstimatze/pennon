package watcher

import "sync"

// MemoryDeduper is a process-local Deduper: an in-memory set of delivery
// IDs, guarded by a mutex so Seen's check-and-set is atomic under
// concurrent webhook deliveries. It never evicts, so a long-running
// process accumulates one entry per delivery for its whole lifetime —
// fine for now since nothing durable depends on the watcher surviving a
// restart yet (see docs/ARCHITECTURE.md "The watcher"); revisit once the
// watcher is actually deployed against a live Linear webhook.
type MemoryDeduper struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

// NewMemoryDeduper returns a ready-to-use MemoryDeduper.
func NewMemoryDeduper() *MemoryDeduper {
	return &MemoryDeduper{seen: make(map[string]struct{})}
}

// Seen implements Deduper.
func (d *MemoryDeduper) Seen(deliveryID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.seen[deliveryID]; ok {
		return true, nil
	}
	d.seen[deliveryID] = struct{}{}
	return false, nil
}
