package settings

import "testing"

func TestCatalogOrderIsContract(t *testing.T) {
	// P0007 3.7 의 안내 목록 순서이자 dump 출력 순서다. 앞뒤와 개수를 못 박아 둔다.
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
	// 부속 인수가 필수인 항목은 AppExit 과 AppEvents 둘뿐이다.
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
	// L0008 2.3 규칙 3: 기본값이 정의되지 않은 항목은 빈 값을 답한다.
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
	// 기본값(1500)을 명시적으로 지정하면 값이 저장되지 않고 지워진다.
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

	// 기본값이 빈 문자열인 항목은 빈 값을 저장 요청으로 본다.
	// 빈 값을 곧바로 삭제로 바꾸면 "명시적 빈 값"을 표현할 길이 사라진다.
	app, _ := Lookup("Application")
	if got := PlanWriteString(app, ""); got != ResultSet {
		t.Errorf("PlanWriteString(empty default) = %v, want ResultSet", got)
	}
}

func TestPlanClear(t *testing.T) {
	// 문자열 항목에 기본값이 있으면 그 값을 다시 쓸 대상으로 삼는다.
	exit, _ := Lookup("AppExit")
	if v, ok := PlanClear(exit); !ok || v != "Restart" {
		t.Errorf("PlanClear(AppExit) = %q, %v; want Restart, true", v, ok)
	}
	// 숫자 항목은 곧바로 지운다.
	throttleSetting, _ := Lookup("AppThrottle")
	if _, ok := PlanClear(throttleSetting); ok {
		t.Error("PlanClear(AppThrottle) = has rewrite, want direct delete")
	}
	// 기본값이 없는 항목도 곧바로 지운다.
	aff, _ := Lookup("AppAffinity")
	if _, ok := PlanClear(aff); ok {
		t.Error("PlanClear(AppAffinity) = has rewrite, want direct delete")
	}
}
