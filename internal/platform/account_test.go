package platform

import "testing"

func TestSpecialServiceAccountRecognizesBuiltInAccounts(t *testing.T) {
	for _, account := range []string{
		"LocalSystem",
		"localsystem",
		`NT Authority\LocalService`,
		`NT Authority\NetworkService`,
		`NT Service\svc`,
	} {
		if !SpecialServiceAccount("svc", account) {
			t.Errorf("SpecialServiceAccount(%q) = false, want true", account)
		}
	}
	if SpecialServiceAccount("svc", `DOMAIN\user`) {
		t.Error(`SpecialServiceAccount("DOMAIN\\user") = true, want false`)
	}
	if SpecialServiceAccount("svc", `NT Service\other`) {
		t.Error(`SpecialServiceAccount("NT Service\\other") = true, want false (virtual account of a different service)`)
	}
}
