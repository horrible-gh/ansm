package settings

import "testing"

func TestCatalogOrderIsContract(t *testing.T) {
	// This section follows the documented behavioral contract. See P0007 3.7.
	names := Names()
	if len(names) != 49 {
		t.Fatalf("len(Names()) = %d, want 49", len(names))
	}
	if names[0] != "Application" {
		t.Errorf("first = %q, want Application", names[0])
	}
	if names[len(names)-1] != "Type" {
		t.Errorf("last = %q, want Type", names[len(names)-1])
	}
	// required follows the documented behavioral contract. See AppExit, AppEvents.
	var required []string
	for _, s := range All() {
		if s.RequiresSub {
			required = append(required, s.Name)
		}
	}
	if len(required) != 2 || required[0] != "AppExit" || required[1] != "AppEvents" {
		t.Errorf("RequiresSub = %v, want [AppExit AppEvents]", required)
	}
}

func TestLookupIgnoresCase(t *testing.T) {
	if s, ok := Lookup("appthrottle"); !ok || s.Name != "AppThrottle" {
		t.Errorf("Lookup(appthrottle) = %+v, %v", s, ok)
	}
	if _, ok := Lookup("AppNoSuchThing"); ok {
		t.Error("Lookup(AppNoSuchThing) = ok, want not found")
	}
}

func TestNoDefaultSettings(t *testing.T) {
	// for follows the documented behavioral contract. See L0008 2.3.
	for _, name := range []string{"AppAffinity", "AppStdin", "AppEnvironment", "AppEnvironmentExtra", "DependOnService", "DependOnGroup"} {
		s, ok := Lookup(name)
		if !ok {
			t.Fatalf("%s not found", name)
		}
		if s.HasDefault {
			t.Errorf("%s HasDefault = true, want false", name)
		}
	}
}

func TestPlanWriteNumberDeletesWhenEqualToDefault(t *testing.T) {
	s, _ := Lookup("AppThrottle")
	// if follows the documented behavioral contract.
	if got := PlanWriteNumber(s, 1500); got != ResultReset {
		t.Errorf("PlanWriteNumber(1500) = %v, want ResultReset", got)
	}
	if got := PlanWriteNumber(s, 3000); got != ResultSet {
		t.Errorf("PlanWriteNumber(3000) = %v, want ResultSet", got)
	}
}

func TestPlanWriteStringIgnoresCaseAndSkipsEmptyDefault(t *testing.T) {
	priority, _ := Lookup("AppPriority")
	if got := PlanWriteString(priority, "normal_priority_class"); got != ResultReset {
		t.Errorf("PlanWriteString(default, other case) = %v, want ResultReset", got)
	}
	if got := PlanWriteString(priority, "HIGH_PRIORITY_CLASS"); got != ResultSet {
		t.Errorf("PlanWriteString(non-default) = %v, want ResultSet", got)
	}

	// This section follows the documented behavioral contract.
	app, _ := Lookup("Application")
	if got := PlanWriteString(app, ""); got != ResultSet {
		t.Errorf("PlanWriteString(empty default) = %v, want ResultSet", got)
	}
}

func TestPlanClear(t *testing.T) {
	// This section follows the documented behavioral contract.
	exit, _ := Lookup("AppExit")
	if v, ok := PlanClear(exit); !ok || v != "Restart" {
		t.Errorf("PlanClear(AppExit) = %q, %v; want Restart, true", v, ok)
	}
	// This section follows the documented behavioral contract.
	throttleSetting, _ := Lookup("AppThrottle")
	if _, ok := PlanClear(throttleSetting); ok {
		t.Error("PlanClear(AppThrottle) = has rewrite, want direct delete")
	}
	// This section follows the documented behavioral contract.
	aff, _ := Lookup("AppAffinity")
	if _, ok := PlanClear(aff); ok {
		t.Error("PlanClear(AppAffinity) = has rewrite, want direct delete")
	}
}
