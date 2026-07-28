package messages

import (
	"fmt"
	"strings"
)

// 콘솔 문구는 원본 영어 문구를 그대로 쓴다(P0007 0장: 다국어 문구 유지).
// 여기 있는 것은 T2(골격)까지 실제로 쓰이는 문구뿐이며, 나머지는 각 단계에서 채운다.
const (
	textInvalidParameter    = "Invalid parameter \"%s\".  Valid parameters are:"
	textMissingSubparameter = "Parameter \"%s\" requires a subparameter!"
	textSetSetting          = "Set parameter \"%s\" for service \"%s\"."
	textResetSetting        = "Reset parameter \"%s\" for service \"%s\"."
	textInvalidExitAction   = "Invalid exit action \"%s\"!"
	textInvalidHookEvent    = "Invalid hook event!"
	textInvalidHookAction   = "Invalid hook action for hook event \"%s\"!"
	textInvalidHookName     = "Invalid hook name!"
	textBadControlResponse  = "%s: Unexpected status %s in response to %s control."
	textControlSucceeded    = "%s: %s: The operation completed successfully."
	textPreRemoveService    = "To remove a service without confirmation:\r\n\r\n\t%s remove <servicename> confirm"
)

// InvalidParameterText 는 잘못된 항목 이름 안내와 전체 목록을 조립한다(P0007 3.7).
// names 의 순서가 곧 계약이므로 정렬하지 않는다.
func InvalidParameterText(name string, names []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, textInvalidParameter, name)
	for _, n := range names {
		b.WriteString("\r\n")
		b.WriteString(n)
	}
	return b.String()
}

// MissingSubparameterText 는 부속 인수 누락 안내다(P0007 3.8).
func MissingSubparameterText(name string) string {
	return fmt.Sprintf(textMissingSubparameter, name)
}

// SetSettingText 와 ResetSettingText 는 값 쓰기 결과 문구다(P0007 3.4·3.5).
func SetSettingText(param, service string) string {
	return fmt.Sprintf(textSetSetting, param, service)
}

func ResetSettingText(param, service string) string {
	return fmt.Sprintf(textResetSetting, param, service)
}

// InvalidExitActionText 는 알 수 없는 종료 조치 안내와 유효한 값 목록을 조립한다.
func InvalidExitActionText(value string, valid []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, textInvalidExitAction, value)
	for _, v := range valid {
		b.WriteString("\r\n")
		b.WriteString(v)
	}
	return b.String()
}

// InvalidHookEventText 는 유효한 사건 목록과 함께 안내한다(P0007 3.10).
func InvalidHookEventText(events []string) string {
	var b strings.Builder
	b.WriteString(textInvalidHookEvent)
	for _, e := range events {
		b.WriteString("\r\n")
		b.WriteString(e)
	}
	return b.String()
}

// InvalidHookActionText 는 그 사건에서 쓸 수 있는 동작 목록과 함께 안내한다.
func InvalidHookActionText(event string, actions []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, textInvalidHookAction, event)
	for _, a := range actions {
		b.WriteString("\r\n")
		b.WriteString(a)
	}
	return b.String()
}

// InvalidHookNameText 는 '/' 형식이 아닌 훅 이름 안내다.
func InvalidHookNameText() string { return textInvalidHookName }

// ControlSucceededText 는 제어가 원하는 상태에 도달했을 때의 문구다(P0007 4.1).
func ControlSucceededText(service, control string) string {
	return fmt.Sprintf(textControlSucceeded, service, control)
}

// BadControlResponseText 는 예상 밖 상태 문구다(P0007 4.1).
func BadControlResponseText(service, status, control string) string {
	return fmt.Sprintf(textBadControlResponse, service, status, control)
}

// PreRemoveServiceText 는 confirm 없이 remove 를 부른 경우의 안내다(P0007 2.6).
func PreRemoveServiceText(exe string) string {
	return fmt.Sprintf(textPreRemoveService, exe)
}
