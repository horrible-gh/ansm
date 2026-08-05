package platform

import (
	"syscall"
	"testing"
)

func TestIsServiceNotActive(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"bare errno", ErrServiceNotActive, true},
		{"wrapped in *Error", &Error{Code: 1, Op: "control service", Err: ErrServiceNotActive}, true},
		{"different errno", syscall.Errno(5), false},
		{"nil", nil, false},
	}
	for _, tt := range cases {
		if got := IsServiceNotActive(tt.err); got != tt.want {
			t.Errorf("%s: IsServiceNotActive(%v) = %v, want %v", tt.name, tt.err, got, tt.want)
		}
	}
}
