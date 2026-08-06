package platform

import "strings"

// SpecialServiceAccount reports whether account is one of the built-in
// virtual accounts that never require a password: LocalSystem, the two
// NT Authority pseudo-accounts, or the service's own NT Service\<service>
// virtual account. GUI validation (internal/gui) and the Windows account
// grant path (account_windows.go) share this single check so the two
// layers agree on what counts as "special" for a given service.
func SpecialServiceAccount(service, account string) bool {
	return strings.EqualFold(account, "LocalSystem") ||
		strings.EqualFold(account, `NT Authority\LocalService`) ||
		strings.EqualFold(account, `NT Authority\NetworkService`) ||
		strings.EqualFold(account, `NT Service\`+service)
}
