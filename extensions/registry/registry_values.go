package registry

import (
	"fmt"
	"strings"
)

// normalizeType accepts both Go-style ("dword") and Win32-style ("REG_DWORD") type names.
func normalizeType(t string) string {
	s := strings.ToLower(t)
	s = strings.TrimPrefix(s, "reg_")
	switch s {
	case "sz", "string", "":
		return "sz"
	case "expand_sz", "expandstring":
		return "expandstring"
	case "multi_sz", "multistring":
		return "multistring"
	default:
		return s
	}
}

// formatDesired renders r.Data into the exact string form that readValue
// produces for the value already stored in the registry (decimal for
// dword/qword, comma-joined for multistring, lowercase hex for binary, %v
// otherwise). Check compares this against the current value so a correctly-set
// value reports InSync instead of churning on every run.
//
// It returns an error when r.Data's Go type cannot represent the configured
// registry type (e.g. a string supplied for a DWORD), so a misconfigured
// hardening value fails loudly rather than silently writing 0/fallback.
func (r *Registry) formatDesired() (string, error) {
	switch normalizeType(r.Type) {
	case "dword":
		v, err := toUint32(r.Data)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", v), nil
	case "qword":
		v, err := toUint64(r.Data)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", v), nil
	case "multistring":
		v, err := toStringSlice(r.Data)
		if err != nil {
			return "", err
		}
		return strings.Join(v, ","), nil
	case "binary":
		v, err := toBytes(r.Data)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%x", v), nil
	default:
		return fmt.Sprintf("%v", r.Data), nil
	}
}

// toUint32 converts r.Data to a DWORD, erroring on non-integer types instead of
// silently coercing them to 0.
func toUint32(v any) (uint32, error) {
	switch n := v.(type) {
	case int:
		return uint32(n), nil
	case int8:
		return uint32(n), nil
	case int16:
		return uint32(n), nil
	case int32:
		return uint32(n), nil
	case int64:
		return uint32(n), nil
	case uint:
		return uint32(n), nil
	case uint8:
		return uint32(n), nil
	case uint16:
		return uint32(n), nil
	case uint32:
		return n, nil
	case uint64:
		return uint32(n), nil
	case float32:
		return uint32(n), nil
	case float64:
		return uint32(n), nil
	default:
		return 0, fmt.Errorf("registry DWORD data must be an integer, got %T", v)
	}
}

// toUint64 converts r.Data to a QWORD, erroring on non-integer types instead of
// silently coercing them to 0.
func toUint64(v any) (uint64, error) {
	switch n := v.(type) {
	case int:
		return uint64(n), nil
	case int8:
		return uint64(n), nil
	case int16:
		return uint64(n), nil
	case int32:
		return uint64(n), nil
	case int64:
		return uint64(n), nil
	case uint:
		return uint64(n), nil
	case uint8:
		return uint64(n), nil
	case uint16:
		return uint64(n), nil
	case uint32:
		return uint64(n), nil
	case uint64:
		return n, nil
	case float32:
		return uint64(n), nil
	case float64:
		return uint64(n), nil
	default:
		return 0, fmt.Errorf("registry QWORD data must be an integer, got %T", v)
	}
}

// toStringSlice converts r.Data to a MULTI_SZ slice, erroring on unsupported
// types instead of silently stringifying them.
func toStringSlice(v any) ([]string, error) {
	switch s := v.(type) {
	case []string:
		return s, nil
	case string:
		return strings.Split(s, ","), nil
	case []any:
		out := make([]string, len(s))
		for i, e := range s {
			str, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("registry MULTI_SZ data element %d must be a string, got %T", i, e)
			}
			out[i] = str
		}
		return out, nil
	default:
		return nil, fmt.Errorf("registry MULTI_SZ data must be a string or []string, got %T", v)
	}
}

// toBytes converts r.Data to BINARY bytes, erroring on unsupported types instead
// of silently stringifying them.
func toBytes(v any) ([]byte, error) {
	switch b := v.(type) {
	case []byte:
		return b, nil
	case string:
		return []byte(b), nil
	default:
		return nil, fmt.Errorf("registry BINARY data must be a string or []byte, got %T", v)
	}
}
