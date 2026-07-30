package processtree

import "testing"

func TestIsRealChildUsesParentLifetime(t *testing.T) {
	tests := []struct {
		name                                 string
		parent, listedParent                 uint32
		parentCreated, parentExited, created uint64
		want                                 bool
	}{
		{"inside lifetime", 10, 10, 100, 300, 200, true},
		{"different parent", 10, 11, 100, 300, 200, false},
		{"created before parent", 10, 10, 100, 300, 99, false},
		{"created after parent exit", 10, 10, 100, 300, 301, false},
		{"unknown parent creation", 10, 10, 0, 300, 200, false},
		{"unknown parent exit", 10, 10, 100, 0, 200, false},
		{"unknown child creation", 10, 10, 100, 300, 0, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := IsRealChild(test.parent, test.listedParent, test.parentCreated, test.parentExited, test.created)
			if got != test.want {
				t.Fatalf("IsRealChild() = %v, want %v", got, test.want)
			}
		})
	}
}
