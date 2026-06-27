package registry

import "testing"

func TestNormalizeType(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "sz"},
		{"sz", "sz"},
		{"string", "sz"},
		{"REG_SZ", "sz"},
		{"dword", "dword"},
		{"REG_DWORD", "dword"},
		{"QWORD", "qword"},
		{"expandstring", "expandstring"},
		{"REG_EXPAND_SZ", "expandstring"},
		{"multistring", "multistring"},
		{"REG_MULTI_SZ", "multistring"},
		{"binary", "binary"},
		{"REG_BINARY", "binary"},
	}
	for _, tt := range tests {
		if got := normalizeType(tt.in); got != tt.want {
			t.Errorf("normalizeType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestToUint32(t *testing.T) {
	ok := []struct {
		in   any
		want uint32
	}{
		{int(1), 1},
		{int32(2), 2},
		{int64(3), 3},
		{uint32(4), 4},
		{uint64(5), 5},
		{float64(6), 6},
	}
	for _, tt := range ok {
		got, err := toUint32(tt.in)
		if err != nil {
			t.Errorf("toUint32(%v): unexpected error %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("toUint32(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}

	// Mismatched/unhandled types must error instead of coercing to 0.
	for _, bad := range []any{"1", []byte{0x01}, []string{"a"}, nil, struct{}{}} {
		if got, err := toUint32(bad); err == nil {
			t.Errorf("toUint32(%#v) = %d, want error", bad, got)
		}
	}
}

func TestToUint64(t *testing.T) {
	ok := []struct {
		in   any
		want uint64
	}{
		{int(1), 1},
		{int32(2), 2},
		{int64(3), 3},
		{uint32(4), 4},
		{uint64(5), 5},
		{float64(6), 6},
	}
	for _, tt := range ok {
		got, err := toUint64(tt.in)
		if err != nil {
			t.Errorf("toUint64(%v): unexpected error %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("toUint64(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}

	for _, bad := range []any{"1", []byte{0x01}, []string{"a"}, nil, struct{}{}} {
		if got, err := toUint64(bad); err == nil {
			t.Errorf("toUint64(%#v) = %d, want error", bad, got)
		}
	}
}

func TestToStringSlice(t *testing.T) {
	tests := []struct {
		in   any
		want []string
	}{
		{[]string{"a", "b"}, []string{"a", "b"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{[]any{"x", "y"}, []string{"x", "y"}},
	}
	for _, tt := range tests {
		got, err := toStringSlice(tt.in)
		if err != nil {
			t.Errorf("toStringSlice(%#v): unexpected error %v", tt.in, err)
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("toStringSlice(%#v) = %#v, want %#v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("toStringSlice(%#v)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
			}
		}
	}

	for _, bad := range []any{1, []byte{0x01}, []any{1, 2}, nil} {
		if _, err := toStringSlice(bad); err == nil {
			t.Errorf("toStringSlice(%#v) = nil error, want error", bad)
		}
	}
}

func TestToBytes(t *testing.T) {
	if got, err := toBytes([]byte{0x01, 0x02}); err != nil || string(got) != "\x01\x02" {
		t.Errorf("toBytes([]byte) = %#v, %v", got, err)
	}
	if got, err := toBytes("ab"); err != nil || string(got) != "ab" {
		t.Errorf("toBytes(string) = %#v, %v", got, err)
	}
	for _, bad := range []any{1, []string{"a"}, nil, struct{}{}} {
		if _, err := toBytes(bad); err == nil {
			t.Errorf("toBytes(%#v) = nil error, want error", bad)
		}
	}
}

// TestFormatDesired_MirrorsReadValue verifies that formatDesired renders desired
// data in the same string form readValue produces, so a correctly-set value
// reports InSync. The expected strings below mirror readValue exactly: decimal
// for dword/qword, comma-joined for multistring, lowercase hex for binary.
func TestFormatDesired_MirrorsReadValue(t *testing.T) {
	tests := []struct {
		name string
		typ  string
		data any
		want string
	}{
		{"dword int", "dword", 1, "1"},
		{"dword float64", "REG_DWORD", float64(1), "1"},
		{"qword int64", "qword", int64(42), "42"},
		{"sz string", "sz", "hello", "hello"},
		{"sz default empty type", "", "hello", "hello"},
		{"expandstring", "expandstring", "%PATH%", "%PATH%"},
		{"multistring slice", "multistring", []string{"a", "b"}, "a,b"},
		{"multistring csv string", "REG_MULTI_SZ", "a,b", "a,b"},
		{"binary bytes", "binary", []byte{0x01, 0x0a, 0xff}, "010aff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Registry{Type: tt.typ, Data: tt.data}
			got, err := r.formatDesired()
			if err != nil {
				t.Fatalf("formatDesired() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("formatDesired() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatDesired_TypeMismatchFailsLoudly ensures a value whose Go type cannot
// represent the configured registry type returns an error rather than silently
// formatting (and later writing) a wrong/permissive value.
func TestFormatDesired_TypeMismatchFailsLoudly(t *testing.T) {
	tests := []struct {
		name string
		typ  string
		data any
	}{
		{"string for dword", "dword", "1"},
		{"string for qword", "qword", "1"},
		{"int for multistring", "multistring", 5},
		{"int for binary", "binary", 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Registry{Type: tt.typ, Data: tt.data}
			if got, err := r.formatDesired(); err == nil {
				t.Errorf("formatDesired() = %q, want error for %s", got, tt.name)
			}
		})
	}
}
