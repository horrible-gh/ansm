// Package quote implements the documented contracts for this component. See Package, L0008 2.7.
package quote

import (
	"errors"
	"strings"

	"ansm/internal/params"
)

// ErrTooLong follows the documented behavioral contract. See ErrTooLong, L0008 2.7.
var ErrTooLong = errors.New("quoted value exceeds buffer")

// bufferLimit follows the documented behavioral contract.
const bufferLimit = params.ValueMax * 2

// QuoteLimited follows the documented behavioral contract. See QuoteLimited, Quote, ErrTooLong.
func QuoteLimited(s string) (string, error) {
	q := Quote(s)
	if len(q) > bufferLimit {
		return "", ErrTooLong
	}
	return q, nil
}

// escapeChars follows the documented behavioral contract.
const escapeChars = `"&%^<>|`

// quoteOnlyChars follows the documented behavioral contract.
const quoteOnlyChars = " \t\n\v*"

func containsAny(s, set string) bool {
	return strings.ContainsAny(s, set)
}

// Quote follows the documented behavioral contract. See Quote, L0008 2.7, Windows.
func Quote(s string) string {
	// if follows the documented behavioral contract. See L0008 2.7, P0007 3.6.
	if len(s) == 0 {
		return `""`
	}

	needEscape := containsAny(s, escapeChars)
	needQuote := needEscape || containsAny(s, quoteOnlyChars)
	if !needQuote {
		return s
	}

	var b strings.Builder
	if needEscape {
		b.WriteByte('^')
	}
	b.WriteByte('"')

	for i := 0; i < len(s); {
		n := 0
		for i < len(s) && s[i] == '\\' {
			i++
			n++
		}

		if i == len(s) {
			// This section follows the documented behavioral contract.
			emitBackslashes(&b, 2*n, needEscape)
			break
		}

		if s[i] == '"' {
			emitBackslashes(&b, 2*n+1, needEscape)
			if needEscape {
				b.WriteByte('^')
			}
			b.WriteByte('"')
		} else {
			emitBackslashes(&b, n, needEscape)
			if needEscape && strings.IndexByte(escapeChars, s[i]) >= 0 {
				b.WriteByte('^')
			}
			b.WriteByte(s[i])
		}
		i++
	}

	if needEscape {
		b.WriteByte('^')
	}
	b.WriteByte('"')
	return b.String()
}

func emitBackslashes(b *strings.Builder, count int, needEscape bool) {
	for i := 0; i < count; i++ {
		if needEscape {
			b.WriteByte('^')
		}
		b.WriteByte('\\')
	}
}
