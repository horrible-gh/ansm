package platform

import (
	"strings"
	"testing"
)

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

func TestIsUPNFormatDetectsUserAtDomain(t *testing.T) {
	cases := map[string]bool{
		"user@domain.example":        true,
		`DOMAIN\user`:                false,
		`DOMAIN\user@domain.example`: false,
		"LocalSystem":                false,
		"":                           false,
		"user@":                      false,
		"@domain.example":            false,
	}
	for account, want := range cases {
		if got := isUPNFormat(account); got != want {
			t.Errorf("isUPNFormat(%q) = %v, want %v", account, got, want)
		}
	}
}

func TestGrantLogonAsServiceRejectsUPNWithClearError(t *testing.T) {
	const upn = "svcuser@example.test"
	err := grantLogonAsService(upn)
	if err == nil {
		t.Fatal("expected an error for a UPN-format account")
	}
	if !strings.Contains(err.Error(), upn) || !strings.Contains(err.Error(), "UPN") {
		t.Errorf("error = %q, want it to name the account and mention UPN", err.Error())
	}
}

func TestGrantLogonAsServiceErrorNamesTheAttemptedAccount(t *testing.T) {
	const bogus = "no-such-account-ansm-regression-test"
	err := grantLogonAsService(bogus)
	if err == nil {
		t.Skip("bogus account unexpectedly resolved on this machine")
	}
	if !strings.Contains(err.Error(), bogus) {
		t.Errorf("error = %q, want it to include the attempted account %q", err.Error(), bogus)
	}
}
