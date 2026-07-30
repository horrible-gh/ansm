package affinity

import (
	"strconv"
	"testing"
)

func TestParseMask(t *testing.T) {
	tests := []struct {
		in   string
		want uint64
	}{
		{"", 0},
		{"0", 1},
		{"0,1", 0b11},
		{"0-2", 0b111},
		{"0,2-5,7", 0b10111101},
		{"63", 1 << 63},
		{" 0 , 2 ", 0b101}, // Follows the documented contract.
	}
	for _, tc := range tests {
		got, err := ParseMask(tc.in)
		if err != nil {
			t.Errorf("ParseMask(%q) error = %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseMask(%q) = %#b, want %#b", tc.in, got, tc.want)
		}
	}
}

func TestParseMaskErrors(t *testing.T) {
	// if follows the documented behavioral contract. See L0008 5.1.
	if _, err := ParseMask("64"); err != ErrOutOfRange {
		t.Errorf(`ParseMask("64") = %v, want ErrOutOfRange`, err)
	}
	for _, in := range []string{"0-", "0,,1", "x", "3-1", "-1"} {
		if _, err := ParseMask(in); err == nil {
			t.Errorf("ParseMask(%q) = nil error, want failure", in)
		}
	}
}

func TestFormatMask(t *testing.T) {
	tests := []struct {
		in   uint64
		want string
	}{
		{0, ""},
		{1, "0"},
		// This section follows the documented behavioral contract.
		{0b11, "0,1"},
		// This section follows the documented behavioral contract.
		{0b111, "0-2"},
		{0b10111101, "0,2-5,7"},
		{1 << 63, "63"},
	}
	for _, tc := range tests {
		if got := FormatMask(tc.in); got != tc.want {
			t.Errorf("FormatMask(%#b) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	for _, s := range []string{"", "0", "0,1", "0-2", "0,2-5,7", "1,3,5", "63"} {
		mask, err := ParseMask(s)
		if err != nil {
			t.Fatalf("ParseMask(%q): %v", s, err)
		}
		if got := FormatMask(mask); got != s {
			t.Errorf("round trip %q -> %#b -> %q", s, mask, got)
		}
	}
}

func TestEffective(t *testing.T) {
	// if follows the documented behavioral contract.
	if got, changed := Effective(0, 0b1111); got != 0 || changed {
		t.Errorf("Effective(0, ...) = %#b, %v; want 0, false", got, changed)
	}
	// if follows the documented behavioral contract. See CPU.
	if got, changed := Effective(0b1111, 0b0011); got != 0b0011 || !changed {
		t.Errorf("Effective = %#b, %v; want 0b11, true", got, changed)
	}
	// if follows the documented behavioral contract.
	if got, changed := Effective(0b0011, 0b1111); got != 0b0011 || changed {
		t.Errorf("Effective = %#b, %v; want 0b11, false", got, changed)
	}
}

// TestApplicableCutsTheMaskToTheBuildWidth follows the documented behavioral contract. See SetProcessAffinityMask.
func TestApplicableCutsTheMaskToTheBuildWidth(t *testing.T) {
	for _, tc := range []struct {
		mask    uint64
		width   int
		applied uint64
		dropped bool
	}{
		{mask: 0b1011, width: 32, applied: 0b1011},
		{mask: 1 << 31, width: 32, applied: 1 << 31},
		{mask: 1 << 32, width: 32, applied: 0, dropped: true},
		{mask: 1<<32 | 0b11, width: 32, applied: 0b11, dropped: true},
		{mask: 1 << 63, width: 64, applied: 1 << 63},
		{mask: ^uint64(0), width: 64, applied: ^uint64(0)},
		{mask: ^uint64(0), width: 32, applied: 0xffffffff, dropped: true},
		{mask: 0, width: 32, applied: 0},
	} {
		applied, dropped := Applicable(tc.mask, tc.width)
		if applied != tc.applied || dropped != tc.dropped {
			t.Errorf("Applicable(%#x, %d) = %#x, %v; want %#x, %v",
				tc.mask, tc.width, applied, dropped, tc.applied, tc.dropped)
		}
	}
}

func TestMaskWidthMatchesThePointerSize(t *testing.T) {
	want := 32
	if strconv.IntSize == 64 {
		want = 64
	}
	if MaskWidth != want {
		t.Errorf("MaskWidth = %d, want %d", MaskWidth, want)
	}
}
