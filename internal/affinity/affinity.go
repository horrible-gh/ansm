// Package affinity implements the documented contracts for this component. See Package, CPU, L0008 2.9, P0007 3.2.
package affinity

import (
	"errors"
	"strconv"
	"strings"

	"ansm/internal/params"
)

var (
	// This section follows the documented behavioral contract. See ErrOutOfRange, CPU.
	ErrOutOfRange = errors.New("cpu number out of range")
	// This section follows the documented behavioral contract. See ErrSyntax.
	ErrSyntax = errors.New("malformed affinity specification")
)

// ParseMask follows the documented behavioral contract. See ParseMask, CPU, Windows, SetProcessAffinityMask, ANSM.
func ParseMask(text string) (uint64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, nil
	}

	var mask uint64
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return 0, ErrSyntax
		}

		first, last, hyphen := strings.Cut(part, "-")
		lo, err := parseCPU(first)
		if err != nil {
			return 0, err
		}
		hi := lo
		if hyphen {
			hi, err = parseCPU(last)
			if err != nil {
				return 0, err
			}
		}
		if hi < lo {
			return 0, ErrSyntax
		}
		for i := lo; i <= hi; i++ {
			mask |= 1 << uint(i)
		}
	}
	return mask, nil
}

func parseCPU(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrSyntax
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, ErrSyntax
	}
	if n < 0 || n >= params.AffinityCPUMax {
		return 0, ErrOutOfRange
	}
	return n, nil
}

// FormatMask follows the documented behavioral contract. See FormatMask, L0008 2.9.
func FormatMask(mask uint64) string {
	if mask == 0 {
		return ""
	}

	var parts []string
	for i := 0; i < params.AffinityCPUMax; {
		if mask&(1<<uint(i)) == 0 {
			i++
			continue
		}
		first := i
		for i < params.AffinityCPUMax && mask&(1<<uint(i)) != 0 {
			i++
		}
		last := i - 1

		switch {
		case first == last:
			parts = append(parts, strconv.Itoa(first))
		case last == first+1:
			parts = append(parts, strconv.Itoa(first), strconv.Itoa(last))
		default:
			parts = append(parts, strconv.Itoa(first)+"-"+strconv.Itoa(last))
		}
	}
	return strings.Join(parts, ",")
}

// MaskWidth follows the documented behavioral contract. See MaskWidth, Win32, SetProcessAffinityMask, ANSM, P0007 3.2.
const MaskWidth = 32 << (^uint(0) >> 63)

// Applicable follows the documented behavioral contract. See Applicable, CPU, ANSM.
func Applicable(mask uint64, width int) (applied uint64, dropped bool) {
	if width <= 0 || width >= 64 {
		return mask, false
	}
	applied = mask & (1<<uint(width) - 1)
	return applied, applied != mask
}

// Effective follows the documented behavioral contract. See Effective, L0008 2.9.
func Effective(wanted, system uint64) (effective uint64, changed bool) {
	if wanted == 0 {
		return 0, false
	}
	effective = wanted & system
	return effective, effective != wanted
}
