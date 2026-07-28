package quote

import "testing"

func TestQuote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// 인용이 필요 없는 값은 그대로 둔다.
		{"plain", `C:\app`, `C:\app`},
		{"digits", `1500`, `1500`},

		// 공백만 있으면 인용만 하고 캐럿은 붙이지 않는다.
		{"space", `My Worker`, `"My Worker"`},
		{"tab", "a\tb", "\"a\tb\""},
		{"star", `*.log`, `"*.log"`},
		{"path with space", `C:\Program Files\app.exe`, `"C:\Program Files\app.exe"`},

		// 빈 문자열은 "" 로 찍는다.
		{"empty", ``, `""`},

		// 탈출 대상이 하나라도 있으면 전체가 ^" 로 감싸이고 각 특수문자에 ^ 가 붙는다.
		{"ampersand", `a&b`, `^"a^&b^"`},
		{"percent", `%PATH%`, `^"^%PATH^%^"`},
		{"pipe", `a|b`, `^"a^|b^"`},

		// 역슬래시는 인용부호 앞에서만 배로 늘린다.
		{"backslash mid", `a\b`, `a\b`},
		{"backslash before quote", `a\"b`, `^"a^\^\^\^"b^"`},

		// 문자열 끝의 역슬래시 무리도 배로 늘린다. 그러지 않으면 닫는
		// 인용부호가 탈출되어 값의 경계가 사라진다.
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
	// 모든 문자가 탈출되는 최악의 값으로도 버퍼 안에 들어가는 길이는 통과해야 한다.
	short := ""
	for i := 0; i < 100; i++ {
		short += "&"
	}
	if _, err := QuoteLimited(short); err != nil {
		t.Fatalf("QuoteLimited(short) = %v, want nil", err)
	}

	// 값 최대 길이의 2배를 넘기면 그 항목만 실패로 처리한다.
	long := make([]byte, bufferLimit)
	for i := range long {
		long[i] = '&'
	}
	if _, err := QuoteLimited(string(long)); err != ErrTooLong {
		t.Fatalf("QuoteLimited(long) = %v, want ErrTooLong", err)
	}
}
