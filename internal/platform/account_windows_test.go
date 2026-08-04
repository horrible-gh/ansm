package platform

import "testing"

func TestNormalizeAccountNameExpandsLocalComputerShorthand(t *testing.T) {
	computer, err := localComputerName()
	if err != nil || computer == "" {
		t.Skip("local computer name is unavailable")
	}
	got := normalizeAccountName(`.\svcuser`)
	want := computer + `\svcuser`
	if got != want {
		t.Errorf("normalizeAccountName(%q) = %q, want %q", `.\svcuser`, got, want)
	}
}

func TestNormalizeAccountNameLeavesOtherFormsUnchanged(t *testing.T) {
	for _, account := range []string{
		"LocalSystem",
		`NT Service\svc`,
		`DOMAIN\user`,
		"plainuser",
		"",
	} {
		if got := normalizeAccountName(account); got != account {
			t.Errorf("normalizeAccountName(%q) = %q, want unchanged", account, got)
		}
	}
}

func TestSpecialServiceAccountRecognizesBuiltInAccounts(t *testing.T) {
	for _, account := range []string{
		"LocalSystem",
		"localsystem",
		`NT Authority\LocalService`,
		`NT Authority\NetworkService`,
		`NT Service\svc`,
	} {
		if !specialServiceAccount("svc", account) {
			t.Errorf("specialServiceAccount(%q) = false, want true", account)
		}
	}
	if specialServiceAccount("svc", `DOMAIN\user`) {
		t.Error(`specialServiceAccount("DOMAIN\\user") = true, want false`)
	}
}
