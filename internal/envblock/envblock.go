// Package envblock implements the documented contracts for this component. See Package, L0008 2.8, REG_MULTI_SZ, NUL, CRLF.
package envblock

import "strings"

// Entry follows the documented behavioral contract. See Entry.
type Entry struct {
	Name  string
	Value string
}

// ParseLines follows the documented behavioral contract. See ParseLines, L0008 2.8.
func ParseLines(lines []string) []Entry {
	var out []Entry
	for _, line := range lines {
		if line == "" {
			continue
		}
		// if follows the documented behavioral contract.
		if line[0] == '=' {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out = append(out, Entry{Name: name, Value: value})
	}
	return out
}

// Parse follows the documented behavioral contract. See Parse, CRLF, LF.
func Parse(text string) []Entry {
	if text == "" {
		return nil
	}
	return ParseLines(splitLines(text))
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(text, "\n")
}

// Format follows the documented behavioral contract. See Format, CRLF, P0007 3.4, REG_MULTI_SZ.
func Format(entries []Entry) string {
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		lines = append(lines, e.Name+"="+e.Value)
	}
	return strings.Join(lines, "\r\n")
}

// Upsert follows the documented behavioral contract. See Upsert, L0008 2.8.
func Upsert(entries []Entry, add Entry) []Entry {
	for i := range entries {
		if strings.EqualFold(entries[i].Name, add.Name) {
			entries[i].Value = add.Value
			return entries
		}
	}
	return append(entries, add)
}

// Remove follows the documented behavioral contract. See Remove, L0008 2.8, KEY, VALUE.
func Remove(entries []Entry, name, value string, matchValue bool) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if strings.EqualFold(e.Name, name) && (!matchValue || e.Value == value) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Merge follows the documented behavioral contract. See Merge, AppEnvironmentExtra.
func Merge(base, extra []Entry) []Entry {
	out := make([]Entry, len(base))
	copy(out, base)
	for _, e := range extra {
		out = Upsert(out, e)
	}
	return out
}

// ExpandPercent follows the documented behavioral contract. See ExpandPercent, NAME.
func ExpandPercent(entries []Entry, value string) string {
	var b strings.Builder
	for len(value) > 0 {
		start := strings.IndexByte(value, '%')
		if start < 0 {
			b.WriteString(value)
			break
		}
		b.WriteString(value[:start])
		value = value[start+1:]
		end := strings.IndexByte(value, '%')
		if end < 0 {
			b.WriteByte('%')
			b.WriteString(value)
			break
		}
		name := value[:end]
		if expanded, ok := Lookup(entries, name); ok {
			b.WriteString(expanded)
		} else {
			b.WriteByte('%')
			b.WriteString(name)
			b.WriteByte('%')
		}
		value = value[end+1:]
	}
	return b.String()
}

// Apply follows the documented behavioral contract. See Apply, NAME, L0008 2.8.
func Apply(base, override []Entry) []Entry {
	out := append([]Entry(nil), base...)
	for _, entry := range override {
		entry.Value = ExpandPercent(out, entry.Value)
		out = Upsert(out, entry)
	}
	return out
}

// Strings follows the documented behavioral contract. See Strings, NAME, VALUE.
func Strings(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name+"="+entry.Value)
	}
	return out
}

// Lookup follows the documented behavioral contract. See Lookup.
func Lookup(entries []Entry, name string) (string, bool) {
	for _, e := range entries {
		if strings.EqualFold(e.Name, name) {
			return e.Value, true
		}
	}
	return "", false
}
