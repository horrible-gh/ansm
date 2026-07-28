// Package affinity 는 CPU 지정 문자열과 비트 마스크를 서로 옮긴다.
//
// L0008 2.9, P0007 3.2 (표기: "0,2-5,7").
package affinity

import (
	"errors"
	"strconv"
	"strings"

	"ansm/internal/params"
)

var (
	// ErrOutOfRange 는 CPU 번호가 0~63 밖이라는 뜻이다. 설정 단계에서 거부한다(경고 1067).
	ErrOutOfRange = errors.New("cpu number out of range")
	// ErrSyntax 는 "0-", "0,,1" 처럼 형식이 어긋났다는 뜻이다. 마스크는 0(전체)으로 되돌린다.
	ErrSyntax = errors.New("malformed affinity specification")
)

// ParseMask 는 "0,2-5,7" 형태를 마스크로 바꾼다.
//
// 빈 문자열은 마스크 0, 곧 "전체 CPU 사용"이다. 이 뜻은 Windows 의
// SetProcessAffinityMask 규약이 아니라 ANSM 내부 표기이므로,
// 실제 적용 직전에 0 인지 확인해 아예 적용하지 않는 쪽으로 처리한다.
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

// FormatMask 는 마스크를 사람이 읽는 표기로 바꾼다.
//
// L0008 2.9: 연속 두 개는 하이픈으로 줄이지 않고 쉼표로 나열하며("0,1"),
// 세 개 이상부터 하이픈을 쓴다("0-2"). 마스크 0 은 빈 문자열이다.
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

// MaskWidth 는 이 빌드가 Win32 에 넘길 수 있는 마스크의 비트 수다.
//
// SetProcessAffinityMask 의 인수는 DWORD_PTR 이라 32비트 빌드에서는 32비트다.
// 마스크 자체는 어느 빌드에서나 64비트로 읽고 쓴다 — 32비트 ANSM 이 64비트
// 기계의 설정을 읽어 보여줄 수 있어야 하기 때문이다(P0007 3.2).
const MaskWidth = 32 << (^uint(0) >> 63)

// Applicable 은 마스크에서 width 비트 안에 들어가는 부분을 떼어 낸다.
//
// dropped 가 true 면 지정한 CPU 가운데 이 빌드가 지정할 수 없는 번호가 있다는
// 뜻이다. 32비트 ANSM 으로 32번 이상의 CPU 를 지정한 경우다. 원본도 같은
// 자리에서 값을 잘라 쓰므로 밖에서 보이는 동작은 같다. 다만 원본은 자른
// 사실을 스스로 알지 못했고, 여기서는 그것을 판정으로 남긴다.
func Applicable(mask uint64, width int) (applied uint64, dropped bool) {
	if width <= 0 || width >= 64 {
		return mask, false
	}
	applied = mask & (1<<uint(width) - 1)
	return applied, applied != mask
}

// Effective 는 지정 마스크와 시스템이 허용하는 마스크의 논리곱을 돌려준다.
//
// L0008 2.9: 결과가 지정과 다르면 호출자가 경고 549 를 남기고 논리곱 결과를 적용한다.
// changed 가 true 면 그 경고 대상이다.
func Effective(wanted, system uint64) (effective uint64, changed bool) {
	if wanted == 0 {
		return 0, false
	}
	effective = wanted & system
	return effective, effective != wanted
}
