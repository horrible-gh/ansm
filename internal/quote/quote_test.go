package quote

import "testing"

func TestQuote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// This section follows the documented behavioral contract.
		{"plain", `C:\app`, `C:\app`},
		{"digits", `1500`, `1500`},

		// This section follows the documented behavioral contract.
		{"space", `My Worker`, `"My Worker"`},
		{"tab", "a\tb", "\"a\tb\""},
		{"star", `*.log`, `"*.log"`},
		{"path with space", `C:\Program Files\app.exe`, `"C:\Program Files\app.exe"`},

		// This section follows the documented behavioral contract.
		{"empty", ``, `""`},

		// This section follows the documented behavioral contract.
		{"ampersand", `a&b`, `^"a^&b^"`},
		{"percent", `%PATH%`, `^"^%PATH^%^"`},
		{"pipe", `a|b`, `^"a^|b^"`},

		// This section follows the documented behavioral contract.
		{"backslash mid", `a\b`, `a\b`},
		{"backslash before quote", `a\"b`, `^"a^\^\^\^"b^"`},

		// This section follows the documented behavioral contract.
		{"trailing backslash with space", `C:\my dir\`, `"C:\my dir\\"`},
		{"trailing two backslashes", `a b\\`, `"a b\\\\"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Quote(tc.in); got != tc.want {
				t.Errorf("Quote(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestQuoteLimited(t *testing.T) {
	// This section follows the documented behavioral contract.
	short := ""
	for i := 0; i < 100; i++ {
		short += "&"
	}
	if _, err := QuoteLimited(short); err != nil {
		t.Fatalf("QuoteLimited(short) = %v, want nil", err)
	}

	// This section follows the documented behavioral contract.
	long := make([]byte, bufferLimit)
	for i := range long {
		long[i] = '&'
	}
	if _, err := QuoteLimited(string(long)); err != ErrTooLong {
		t.Fatalf("QuoteLimited(long) = %v, want ErrTooLong", err)
	}
}
