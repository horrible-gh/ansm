// Package settings defines the complete setting catalog and default-value rules. See P0007 3.1 and L0008 2.3-2.4.
package settings

import "strings"

// Kind follows the documented behavioral contract. See Kind.
type Kind int

const (
	// KindExpandSZ follows the documented behavioral contract. See KindExpandSZ, REG_EXPAND_SZ, L0008 2.4.
	KindExpandSZ Kind = iota
	// This section follows the documented behavioral contract. See KindSZ, REG_SZ.
	KindSZ
	// This section follows the documented behavioral contract. See KindDWORD, REG_DWORD.
	KindDWORD
	// This section follows the documented behavioral contract. See KindMultiSZ, REG_MULTI_SZ, CRLF.
	KindMultiSZ
)

// Numeric follows the documented behavioral contract. See Numeric, L0008 2.3.
func (k Kind) Numeric() bool { return k == KindDWORD }

// Store follows the documented behavioral contract. See Store.
type Store int

const (
	// StoreParameters follows the documented behavioral contract. See StoreParameters, Parameters.
	StoreParameters Store = iota
	// This section follows the documented behavioral contract. See StoreSCM.
	StoreSCM
)

// Setting follows the documented behavioral contract. See Setting.
type Setting struct {
	// Name follows the documented behavioral contract. See Name.
	Name string
	Kind Kind
	// Store follows the documented behavioral contract. See Store, StoreSCM, SCM, API.
	Store Store
	// HasDefault follows the documented behavioral contract. See HasDefault, L0008 2.3.
	HasDefault bool
	// DefaultString follows the documented behavioral contract. See DefaultString.
	DefaultString string
	// DefaultNumber follows the documented behavioral contract. See DefaultNumber.
	DefaultNumber uint32
	// RequiresSub follows the documented behavioral contract. See RequiresSub, AppExit, AppEvents.
	RequiresSub bool
	// DynamicDefault follows the documented behavioral contract. See DynamicDefault, DisplayName, ImagePath.
	DynamicDefault bool
}

// all is ordered deliberately: the same order is used in validation messages and dump output (P0007 3.7).
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

// All follows the documented behavioral contract. See All.
func All() []Setting {
	out := make([]Setting, len(all))
	copy(out, all)
	return out
}

// Names follows the documented behavioral contract. See Names, P0007 3.7, Valid.
func Names() []string {
	out := make([]string, 0, len(all))
	for _, s := range all {
		out = append(out, s.Name)
	}
	return out
}

// Lookup follows the documented behavioral contract. See Lookup.
func Lookup(name string) (Setting, bool) {
	for _, s := range all {
		if strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return Setting{}, false
}

// WriteResult follows the documented behavioral contract. See WriteResult, P0007 3.4.
type WriteResult int

const (
	// ResultSet follows the documented behavioral contract. See ResultSet, Set.
	ResultSet WriteResult = iota
	// This section follows the documented behavioral contract. See ResultReset, Reset.
	ResultReset
)

// PlanWriteString removes a non-empty value when it equals the default case-insensitively. This keeps explicit defaults out of dump output (L0008 2.4).
func PlanWriteString(s Setting, value string) WriteResult {
	if s.HasDefault && s.DefaultString != "" && strings.EqualFold(value, s.DefaultString) {
		return ResultReset
	}
	return ResultSet
}

// PlanWriteNumber applies the numeric default/reset rules from L0008 2.4.
func PlanWriteNumber(s Setting, value uint32) WriteResult {
	if s.HasDefault && value == s.DefaultNumber {
		return ResultReset
	}
	return ResultSet
}

// PlanClear rewrites a defined string default through PlanWriteString so reset operations use one result message; other values are deleted directly.
func PlanClear(s Setting) (rewrite string, hasRewrite bool) {
	if !s.Kind.Numeric() && s.HasDefault && s.DefaultString != "" {
		return s.DefaultString, true
	}
	return "", false
}
