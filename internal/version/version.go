// Package version 은 버전 문자열과 훅·리소스에 넘길 빌드 메타데이터를 담는다.
//
// P0007 2.1 이 정한 표기: "NSSM <버전> <구성> <빌드일자>".
// 채울 값은 P0007 [DEFERRED] 가 T8(패키징)으로 미룬 항목이었다. T8 의 결정은
// 이렇다.
//
//   - 기본값은 이식의 바탕이 된 원본 스냅샷과 같은 값으로 둔다. 아무 것도
//     주입하지 않고 `go build` 만 해도 원본과 같은 한 줄이 나온다.
//
//   - 배포 빌드는 저장소 이력에서 뽑은 값을 링커로 덮어쓴다(tools/dist.ps1).
//
//     go build -ldflags "-X ansm/internal/version.Number=2.24-101-gXXXXXXX \
//     -X ansm/internal/version.BuildDate=2026-07-29"
package version

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

var (
	// Number 는 NSSM_VERSION 환경 변수로도 넘어가는 버전 문자열이다.
	Number = "2.24-101-g897c7ad"
	// BuildDate 는 NSSM_BUILD_DATE 환경 변수로도 넘어가는 빌드 일자다.
	BuildDate = "2017-08-04"
)

// Configuration 은 NSSM_CONFIGURATION 환경 변수 값이다("64-bit" 또는 "32-bit").
func Configuration() string {
	switch runtime.GOARCH {
	case "386", "arm":
		return "32-bit"
	default:
		return "64-bit"
	}
}

// String 은 version 명령이 찍는 한 줄이다.
func String() string {
	return fmt.Sprintf("NSSM %s %s %s", Number, Configuration(), BuildDate)
}

// Numeric 은 버전 문자열에서 네 자리 수 버전을 뽑는다.
//
// 원본 나씀의 version.cmd 와 같은 규칙이다. `git describe --tags --long` 이
// 내는 "2.24-101-g897c7ad" 는 태그 2.24 에서 101 개 커밋 뒤라는 뜻이므로
// 2.24.101.0 이 된다. 태그에 정확히 맞는 빌드는 뒤가 없어 2.24.0.0 이다.
//
// 읽을 수 없는 자리는 0 으로 둔다. 파일 속성 창의 숫자 버전은 참고 값이고,
// 사람이 읽는 값은 Number 그대로 문자열로도 실리기 때문이다.
func Numeric(number string) [4]uint16 {
	var out [4]uint16

	head, rest, _ := strings.Cut(number, "-")
	major, minor, _ := strings.Cut(head, ".")
	out[0] = parseField(major)
	out[1] = parseField(minor)

	if rest != "" {
		count, _, _ := strings.Cut(rest, "-")
		out[2] = parseField(count)
	}
	return out
}

// PreRelease 는 태그에 정확히 맞지 않는 빌드인지 알린다(VS_FF_PRERELEASE).
func PreRelease(number string) bool {
	_, rest, ok := strings.Cut(number, "-")
	if !ok {
		return false
	}
	count, _, _ := strings.Cut(rest, "-")
	return count != "0"
}

func parseField(s string) uint16 {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 16)
	if err != nil {
		return 0
	}
	return uint16(n)
}
