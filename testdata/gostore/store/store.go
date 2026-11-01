package store

// Store keeps items in memory.
type Store struct {
	items map[string]string
}

// New returns an empty store.
func New() *Store { return &Store{items: map[string]string{}} }

// Get returns the item stored under key.
func (s *Store) Get(key string) (string, bool) {
	v, ok := s.items[key]
	return v, ok
}
