// Package quote 는 dump 출력용 인용 규칙을 구현한다.
//
// L0008 2.7. dump 가 찍는 값은 그대로 다시 붙여넣어 실행할 수 있어야 하므로,
// 명령 프롬프트가 삼키는 문자가 섞여 있으면 캐럿(^)까지 함께 붙인다.
package quote

import (
	"errors"
	"strings"

	"ansm/internal/params"
)

// ErrTooLong 은 인용 결과가 대상 버퍼를 넘었다는 뜻이다. L0008 2.7 말미.
// dump 는 이 오류를 만난 항목만 건너뛰고 나머지는 계속 찍은 뒤 종료 코드 1 로 끝낸다.
var ErrTooLong = errors.New("quoted value exceeds buffer")

// bufferLimit 은 값 최대 길이의 2배다. 모든 문자가 탈출되는 최악의 경우를 담는 크기.
const bufferLimit = params.ValueMax * 2

// QuoteLimited 는 Quote 와 같되 결과가 버퍼를 넘으면 ErrTooLong 을 돌려준다.
func QuoteLimited(s string) (string, error) {
	q := Quote(s)
	if len(q) > bufferLimit {
		return "", ErrTooLong
	}
	return q, nil
}

// escapeChars 하나라도 들어 있으면 캐럿 탈출까지 필요하다.
const escapeChars = `"&%^<>|`

// quoteOnlyChars 는 캐럿 없이 인용만 필요하게 만드는 문자다.
const quoteOnlyChars = " \t\n\v*"

func containsAny(s, set string) bool {
	return strings.ContainsAny(s, set)
}

// Quote 는 L0008 2.7 의 quote() 를 그대로 옮긴 것이다.
//
// 역슬래시는 인용부호 앞에서만(그리고 문자열 끝에서만) 배로 늘린다.
// 그 밖의 위치에서는 그대로 둔다. Windows 명령행 해석 규칙과 같은 규칙이다.
func Quote(s string) string {
	// 빈 문자열은 언제나 `""` 다. L0008 2.7 의 의사코드는 길이 검사를 인용 필요
	// 판정 뒤에 두어 빈 값이 그대로 사라지지만, 같은 절과 P0007 3.6 이 본문으로
	// "빈 문자열은 `""` 로 찍는다"고 못 박고 있다. 인용부호 없이 찍으면 그 줄을
	// 다시 붙여넣었을 때 인수 하나가 통째로 없어지므로 본문 쪽 규칙을 따른다.
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
			// 문자열 끝의 역슬래시 무리는 배로 늘린다. 뒤에 닫는 인용부호가
			// 붙으므로 늘리지 않으면 그 인용부호가 탈출되어 버린다.
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
