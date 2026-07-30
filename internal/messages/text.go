package messages

import (
	"fmt"
	"strings"
)

// This section follows the documented behavioral contract. See P0007, T2.
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

// InvalidParameterText follows the documented behavioral contract. See InvalidParameterText, P0007 3.7.
func InvalidParameterText(name string, names []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, textInvalidParameter, name)
	for _, n := range names {
		b.WriteString("\r\n")
		b.WriteString(n)
	}
	return b.String()
}

// MissingSubparameterText follows the documented behavioral contract. See MissingSubparameterText, P0007 3.8.
func MissingSubparameterText(name string) string {
	return fmt.Sprintf(textMissingSubparameter, name)
}

// SetSettingText follows the documented behavioral contract. See SetSettingText, ResetSettingText, P0007 3.4.
func SetSettingText(param, service string) string {
	return fmt.Sprintf(textSetSetting, param, service)
}

func ResetSettingText(param, service string) string {
	return fmt.Sprintf(textResetSetting, param, service)
}

// InvalidExitActionText follows the documented behavioral contract. See InvalidExitActionText.
func InvalidExitActionText(value string, valid []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, textInvalidExitAction, value)
	for _, v := range valid {
		b.WriteString("\r\n")
		b.WriteString(v)
	}
	return b.String()
}

// InvalidHookEventText follows the documented behavioral contract. See InvalidHookEventText, P0007 3.10.
func InvalidHookEventText(events []string) string {
	var b strings.Builder
	b.WriteString(textInvalidHookEvent)
	for _, e := range events {
		b.WriteString("\r\n")
		b.WriteString(e)
	}
	return b.String()
}

// InvalidHookActionText follows the documented behavioral contract. See InvalidHookActionText.
func InvalidHookActionText(event string, actions []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, textInvalidHookAction, event)
	for _, a := range actions {
		b.WriteString("\r\n")
		b.WriteString(a)
	}
	return b.String()
}

// InvalidHookNameText follows the documented behavioral contract. See InvalidHookNameText.
func InvalidHookNameText() string { return textInvalidHookName }

// ControlSucceededText follows the documented behavioral contract. See ControlSucceededText, P0007 4.1.
func ControlSucceededText(service, control string) string {
	return fmt.Sprintf(textControlSucceeded, service, control)
}

// BadControlResponseText follows the documented behavioral contract. See BadControlResponseText, P0007 4.1.
func BadControlResponseText(service, status, control string) string {
	return fmt.Sprintf(textBadControlResponse, service, status, control)
}

// PreRemoveServiceText follows the documented behavioral contract. See PreRemoveServiceText, P0007 2.6.
func PreRemoveServiceText(exe string) string {
	return fmt.Sprintf(textPreRemoveService, exe)
}
