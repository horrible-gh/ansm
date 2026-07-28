// Package settings 는 다룰 수 있는 설정 항목의 전수 목록과 기본값 판정을 담는다.
//
// P0007 3.1 (전수 목록), 3.7 (목록 순서 = dump 출력 순서), L0008 2.3·2.4 (기본값 판정).
//
// 이 목록의 **순서가 계약**이다. 잘못된 항목 이름을 안내할 때 찍는 순서이며,
// 동시에 dump 가 값을 내보내는 순서다.
package settings

import "strings"

// Kind 는 값이 어떤 자료형으로 저장되는지다.
type Kind int

const (
	// KindExpandSZ 는 REG_EXPAND_SZ 다. 문자열은 항상 이 형으로 쓴다(L0008 2.4).
	KindExpandSZ Kind = iota
	// KindSZ 는 REG_SZ 다.
	KindSZ
	// KindDWORD 는 REG_DWORD 다.
	KindDWORD
	// KindMultiSZ 는 REG_MULTI_SZ 다. 사람이 보는 형태는 CRLF 로 이은 목록이다.
	KindMultiSZ
)

// Numeric 은 이 자료형이 숫자인지 알려준다. L0008 2.3·2.4 의 분기 기준이다.
func (k Kind) Numeric() bool { return k == KindDWORD }

// Store 는 값이 어디에 저장되는지다.
type Store int

const (
	// StoreParameters 는 서비스의 Parameters 키다. 이 도구 전용 항목.
	StoreParameters Store = iota
	// StoreSCM 은 서비스 제어 관리자가 직접 들고 있는 구성이다.
	StoreSCM
)

// Setting 은 설정 항목 하나의 계약이다.
type Setting struct {
	// Name 은 레지스트리 값 이름이자 명령행에서 쓰는 항목 이름이다.
	Name string
	Kind Kind
	// Store 가 StoreSCM 이면 SCM API 를 통해 다룬다.
	Store Store
	// HasDefault 가 false 면 기본값이 정의되지 않은 항목이다.
	// L0008 2.3 규칙 3: 이런 항목은 빈 값을 답한다.
	HasDefault bool
	// DefaultString 은 문자열 계열 항목의 기본값이다.
	DefaultString string
	// DefaultNumber 는 숫자 항목의 기본값이다.
	DefaultNumber uint32
	// RequiresSub 가 true 면 부속 인수가 필수다. AppExit 과 AppEvents 둘뿐이다.
	RequiresSub bool
	// DynamicDefault 가 true 면 기본값이 서비스마다 달라 목록에 적을 수 없다는 뜻이다
	// (DisplayName 은 서비스 이름, ImagePath 는 실행 파일 경로).
	DynamicDefault bool
}

// all 의 순서가 P0007 3.7 의 안내 목록 순서이며 dump 출력 순서다. 바꾸면 계약이 깨진다.
var all = []Setting{
	{Name: "Application", Kind: KindExpandSZ, Store: StoreParameters, HasDefault: true, DefaultString: ""},
	{Name: "AppParameters", Kind: KindExpandSZ, Store: StoreParameters, HasDefault: true, DefaultString: ""},
	{Name: "AppDirectory", Kind: KindExpandSZ, Store: StoreParameters, HasDefault: true, DefaultString: ""},
	{Name: "AppExit", Kind: KindSZ, Store: StoreParameters, HasDefault: true, DefaultString: "Restart", RequiresSub: true},
	{Name: "AppEvents", Kind: KindSZ, Store: StoreParameters, HasDefault: true, DefaultString: "", RequiresSub: true},
	{Name: "AppAffinity", Kind: KindSZ, Store: StoreParameters},
	{Name: "AppEnvironment", Kind: KindMultiSZ, Store: StoreParameters},
	{Name: "AppEnvironmentExtra", Kind: KindMultiSZ, Store: StoreParameters},
	{Name: "AppNoConsole", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 0},
	{Name: "AppPriority", Kind: KindSZ, Store: StoreParameters, HasDefault: true, DefaultString: "NORMAL_PRIORITY_CLASS"},
	{Name: "AppRestartDelay", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 0},
	{Name: "AppStdin", Kind: KindExpandSZ, Store: StoreParameters},
	{Name: "AppStdinShareMode", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 2},
	{Name: "AppStdinCreationDisposition", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 3},
	{Name: "AppStdinFlagsAndAttributes", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 128},
	{Name: "AppStdout", Kind: KindExpandSZ, Store: StoreParameters},
	{Name: "AppStdoutShareMode", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 3},
	{Name: "AppStdoutCreationDisposition", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 4},
	{Name: "AppStdoutFlagsAndAttributes", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 128},
	{Name: "AppStdoutCopyAndTruncate", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 0},
	{Name: "AppStderr", Kind: KindExpandSZ, Store: StoreParameters},
	{Name: "AppStderrShareMode", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 3},
	{Name: "AppStderrCreationDisposition", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 4},
	{Name: "AppStderrFlagsAndAttributes", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 128},
	{Name: "AppStderrCopyAndTruncate", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 0},
	{Name: "AppStopMethodSkip", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 0},
	{Name: "AppStopMethodConsole", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 1500},
	{Name: "AppStopMethodWindow", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 1500},
	{Name: "AppStopMethodThreads", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 1500},
	{Name: "AppKillProcessTree", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 1},
	{Name: "AppThrottle", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 1500},
	{Name: "AppRedirectHook", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 0},
	{Name: "AppRotateFiles", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 0},
	{Name: "AppRotateOnline", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 0},
	{Name: "AppRotateSeconds", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 0},
	{Name: "AppRotateBytes", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 0},
	{Name: "AppRotateBytesHigh", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 0},
	{Name: "AppRotateDelay", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 0},
	{Name: "AppTimestampLog", Kind: KindDWORD, Store: StoreParameters, HasDefault: true, DefaultNumber: 0},
	{Name: "DependOnGroup", Kind: KindMultiSZ, Store: StoreSCM},
	{Name: "DependOnService", Kind: KindMultiSZ, Store: StoreSCM},
	{Name: "Description", Kind: KindSZ, Store: StoreSCM, HasDefault: true, DefaultString: ""},
	{Name: "DisplayName", Kind: KindSZ, Store: StoreSCM, DynamicDefault: true},
	{Name: "Environment", Kind: KindMultiSZ, Store: StoreSCM},
	{Name: "ImagePath", Kind: KindExpandSZ, Store: StoreSCM, DynamicDefault: true},
	{Name: "ObjectName", Kind: KindSZ, Store: StoreSCM, HasDefault: true, DefaultString: "LocalSystem"},
	{Name: "Name", Kind: KindSZ, Store: StoreSCM, DynamicDefault: true},
	{Name: "Start", Kind: KindSZ, Store: StoreSCM, DynamicDefault: true},
	{Name: "Type", Kind: KindSZ, Store: StoreSCM, DynamicDefault: true},
}

// All 은 전수 목록을 계약 순서대로 돌려준다.
func All() []Setting {
	out := make([]Setting, len(all))
	copy(out, all)
	return out
}

// Names 는 전수 목록의 이름만 계약 순서대로 돌려준다.
// P0007 3.7 의 "Valid parameters are:" 안내가 이 순서로 찍힌다.
func Names() []string {
	out := make([]string, 0, len(all))
	for _, s := range all {
		out = append(out, s.Name)
	}
	return out
}

// Lookup 은 이름으로 항목을 찾는다. 항목 이름은 대소문자를 구분하지 않는다.
func Lookup(name string) (Setting, bool) {
	for _, s := range all {
		if strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return Setting{}, false
}

// WriteResult 는 값 쓰기의 결과다. 어떤 문구를 낼지 가른다(P0007 3.4·3.5).
type WriteResult int

const (
	// ResultSet 은 값을 저장했다는 뜻이다 → `Set parameter ...`
	ResultSet WriteResult = iota
	// ResultReset 은 값을 지워 기본값으로 되돌렸다는 뜻이다 → `Reset parameter ...`
	ResultReset
)

// PlanWriteString 은 문자열 항목에 값을 쓸 때 저장할지 지울지 판정한다. L0008 2.4.
//
// 기본값이 있고 비어 있지 않으며 대소문자 무시로 값과 같으면 **지운다.**
// 기본값을 명시적으로 지정하면 값이 저장되지 않는 이 성질이,
// dump 가 기본값 항목을 찍지 않아도 되는 근거다.
func PlanWriteString(s Setting, value string) WriteResult {
	if s.HasDefault && s.DefaultString != "" && strings.EqualFold(value, s.DefaultString) {
		return ResultReset
	}
	return ResultSet
}

// PlanWriteNumber 는 숫자 항목에 값을 쓸 때 저장할지 지울지 판정한다. L0008 2.4.
func PlanWriteNumber(s Setting, value uint32) WriteResult {
	if s.HasDefault && value == s.DefaultNumber {
		return ResultReset
	}
	return ResultSet
}

// PlanClear 는 reset/unset 요청의 처리 방식을 판정한다. L0008 2.4 앞부분.
//
// 문자열 항목에 기본값이 정의돼 있으면 그 기본값을 다시 쓸 대상으로 삼는다
// (그리고 PlanWriteString 이 그것을 다시 삭제로 바꾼다 — 결과는 같고 문구만
// `Reset parameter ...` 로 통일된다). 그 밖에는 곧바로 값을 지운다.
func PlanClear(s Setting) (rewrite string, hasRewrite bool) {
	if !s.Kind.Numeric() && s.HasDefault && s.DefaultString != "" {
		return s.DefaultString, true
	}
	return "", false
}
