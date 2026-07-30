// Package rsrc implements the documented contracts for this component. See Package, Windows, COFF, Go, T1, MESSAGETABLE.
package rsrc

import (
	"errors"
	"fmt"
	"sort"
)

// This section follows the documented behavioral contract. See WinUser.
const (
	TypeIcon         uint16 = 3
	TypeMessageTable uint16 = 11
	TypeGroupIcon    uint16 = 14
	TypeVersion      uint16 = 16
	TypeManifest     uint16 = 24
)

// ErrEmpty follows the documented behavioral contract. See ErrEmpty.
var ErrEmpty = errors.New("no resources to write")

// Entry follows the documented behavioral contract. See Entry.
type Entry struct {
	Type     uint16
	Name     uint16
	Language uint16
	Data     []byte
}

// Set follows the documented behavioral contract. See Set.
type Set struct {
	entries []Entry
}

// Add follows the documented behavioral contract. See Add.
func (s *Set) Add(e Entry) error {
	if len(e.Data) == 0 {
		return fmt.Errorf("resource type %d name %d language %d has no data", e.Type, e.Name, e.Language)
	}
	for _, have := range s.entries {
		if have.Type == e.Type && have.Name == e.Name && have.Language == e.Language {
			return fmt.Errorf("duplicate resource type %d name %d language %d", e.Type, e.Name, e.Language)
		}
	}
	s.entries = append(s.entries, e)
	return nil
}

// Entries follows the documented behavioral contract. See Entries, PE.
func (s *Set) Entries() []Entry {
	out := append([]Entry(nil), s.entries...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Language < b.Language
	})
	return out
}
