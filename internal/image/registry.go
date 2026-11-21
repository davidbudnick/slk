package image

import (
	"fmt"
	"image"
	"math/rand/v2"
	"sync"
	"sync/atomic"
)

// Registry mints stable kitty image IDs per (cache key, target cells)
// pair AND tracks whether the bytes for each ID have been confirmed
// delivered to the terminal. Lookup returns fresh=true whenever the
// caller still needs to transmit the image bytes — either because the
// ID is brand new or because a previous Lookup's caller never called
// MarkUploaded (e.g. the OnFlush closure was discarded by a cache
// rebuild before it could fire).
//
// Without this delivery-status tracking, kitty placement cells can
// reference image IDs the terminal never received bytes for, leaving
// affected images as blank cells in the rendered frame.
type Registry struct {
	next     atomic.Uint32
	mu       sync.Mutex
	ids      map[string]uint32
	uploaded map[uint32]bool
}

// NewRegistry constructs a registry. IDs start at 1 (kitty rejects 0).
func NewRegistry() *Registry {
	r := &Registry{
		ids:      map[string]uint32{},
		uploaded: map[uint32]bool{},
	}
	r.next.Store(1)
	return r
}

// placeholderIDBits is how much of an image ID a placeholder cell
// carries, in its foreground colour.
const placeholderIDBits = 24

// newSharedRegistry is the registry of a process that shares its
// terminal: IDs start at a random point in the lower half of what a
// placeholder can carry. A terminal keeps one image per ID, so two slk
// in one terminal (two tmux windows) that both counted from 1 drew each
// other's images.
func newSharedRegistry() *Registry {
	r := NewRegistry()
	r.next.Store(rand.Uint32N(1 << (placeholderIDBits - 1)))
	return r
}

// Lookup returns a stable ID for the given (key, target) pair.
// fresh is true when the caller still needs to transmit the image
// bytes — i.e. the ID is brand new OR no previous caller has called
// MarkUploaded for it yet. Callers that mint a fireable upload
// closure on fresh=true must arrange for MarkUploaded(id) to be
// invoked once the closure runs to completion.
func (r *Registry) Lookup(key string, target image.Point) (id uint32, fresh bool) {
	k := registryKey(key, target)
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.ids[k]; ok {
		return existing, !r.uploaded[existing]
	}
	id = r.next.Add(1)
	r.ids[k] = id
	return id, true
}

// MarkUploaded records that the bytes for id have been successfully
// written to the terminal. Subsequent Lookup calls for the same
// (key, target) will return fresh=false so the caller skips the
// re-encode and re-upload. Safe to call concurrently and idempotent;
// a no-op for unknown IDs (defensive — callers should only invoke
// this for IDs they obtained from a prior Lookup).
func (r *Registry) MarkUploaded(id uint32) {
	r.mu.Lock()
	r.uploaded[id] = true
	r.mu.Unlock()
}

func registryKey(key string, target image.Point) string {
	return fmt.Sprintf("%s|%dx%d", key, target.X, target.Y)
}
