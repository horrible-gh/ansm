// Package version implements the documented contracts for this component. See Package, P0007 2.1, NSSM, P0007, DEFERRED, T8.
package version

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

var (
	// Product is the user-facing application name shown in window titles,
	// message boxes, and usage/version output. The registry layout, event
	// log messages, and hook ABI stay NSSM for compatibility and are not
	// affected by this constant.
	Product = "ANSM"
	// This section follows the documented behavioral contract. See Number, NSSM_VERSION.
	Number = "2.24-101-g897c7ad"
	// This section follows the documented behavioral contract. See BuildDate, NSSM_BUILD_DATE.
	BuildDate = "2017-08-04"
)

// Configuration follows the documented behavioral contract. See Configuration, NSSM_CONFIGURATION.
func Configuration() string {
	switch runtime.GOARCH {
	case "386", "arm":
		return "32-bit"
	default:
		return "64-bit"
	}
}

// String follows the documented behavioral contract. See String.
func String() string {
	return fmt.Sprintf("%s %s %s %s", Product, Number, Configuration(), BuildDate)
}

// Numeric follows the documented behavioral contract. See Numeric, Number.
func Numeric(number string) [4]uint16 {
	var out [4]uint16

	head, rest, _ := strings.Cut(number, "-")
	major, minor, _ := strings.Cut(head, ".")
	out[0] = parseField(major)
	out[1] = parseField(minor)

	if rest != "" {
		count, _, _ := strings.Cut(rest, "-")
		out[2] = parseField(count)
	}
	return out
}

// PreRelease follows the documented behavioral contract. See PreRelease.
func PreRelease(number string) bool {
	_, rest, ok := strings.Cut(number, "-")
	if !ok {
		return false
	}
	count, _, _ := strings.Cut(rest, "-")
	return count != "0"
}

func parseField(s string) uint16 {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 16)
	if err != nil {
		return 0
	}
	return uint16(n)
}
