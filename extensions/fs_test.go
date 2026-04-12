package extensions

import "testing"

// Compile-time check that OSFS satisfies FS.
var _ FS = OSFS{}

func TestRealFS_Nil(t *testing.T) {
	got := RealFS(nil)
	if _, ok := got.(OSFS); !ok {
		t.Errorf("RealFS(nil) should return OSFS, got %T", got)
	}
}

func TestRealFS_NonNil(t *testing.T) {
	custom := OSFS{}
	got := RealFS(custom)
	if got != custom {
		t.Error("RealFS should return the provided FS when non-nil")
	}
}
