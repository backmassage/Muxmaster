package naming

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// CollisionResolver tracks output paths claimed by input files and resolves
// duplicates by appending " - dupN" suffixes. It is safe for sequential use
// within a single pipeline run. All methods are goroutine-safe.
type CollisionResolver struct {
	mu       sync.Mutex
	owners   map[string]string // output path → input path that owns it
	counters map[string]int    // base output path → next dup counter
}

// NewCollisionResolver creates a ready-to-use resolver.
func NewCollisionResolver() *CollisionResolver {
	return &CollisionResolver{
		owners:   make(map[string]string),
		counters: make(map[string]int),
	}
}

// Resolve returns the final output path for input, handling collisions.
// If requestedOutput is unclaimed (or already owned by input), it is returned
// as-is. Otherwise a " - dupN" variant is generated.
func (cr *CollisionResolver) Resolve(input, requestedOutput string) string {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	owner, exists := cr.owners[requestedOutput]
	if !exists || owner == input {
		cr.owners[requestedOutput] = input
		return requestedOutput
	}

	dir := filepath.Dir(requestedOutput)
	base := filepath.Base(requestedOutput)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	counter := cr.counters[requestedOutput]
	if counter == 0 {
		counter = 1
	}

	for {
		candidate := filepath.Join(dir, fmt.Sprintf("%s - dup%d%s", stem, counter, ext))
		cOwner, cExists := cr.owners[candidate]
		if !cExists || cOwner == input {
			cr.counters[requestedOutput] = counter + 1
			cr.owners[candidate] = input
			return candidate
		}
		counter++
	}
}
