package condition

import (
	"context"
	"testing"

	"github.com/TsekNet/converge/extensions"
)

// staticCondition is a test helper that returns a fixed value.
type staticCondition struct {
	met bool
	str string
}

func (s *staticCondition) Met(_ context.Context) (bool, error) { return s.met, nil }
func (s *staticCondition) Wait(_ context.Context) error         { return nil }
func (s *staticCondition) String() string                       { return s.str }

func met(name string) extensions.Condition  { return &staticCondition{met: true, str: name} }
func unmet(name string) extensions.Condition { return &staticCondition{met: false, str: name} }

func TestAll(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		conds   []extensions.Condition
		wantMet bool
	}{
		{"all met", []extensions.Condition{met("a"), met("b")}, true},
		{"one unmet", []extensions.Condition{met("a"), unmet("b")}, false},
		{"all unmet", []extensions.Condition{unmet("a"), unmet("b")}, false},
		{"single met", []extensions.Condition{met("a")}, true},
		{"empty", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := All(tt.conds...)
			got, err := c.Met(ctx)
			if err != nil {
				t.Fatalf("Met() error = %v", err)
			}
			if got != tt.wantMet {
				t.Errorf("Met() = %v, want %v", got, tt.wantMet)
			}
		})
	}
}

func TestAny(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		conds   []extensions.Condition
		wantMet bool
	}{
		{"all met", []extensions.Condition{met("a"), met("b")}, true},
		{"one met", []extensions.Condition{unmet("a"), met("b")}, true},
		{"all unmet", []extensions.Condition{unmet("a"), unmet("b")}, false},
		{"single met", []extensions.Condition{met("a")}, true},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Any(tt.conds...)
			got, err := c.Met(ctx)
			if err != nil {
				t.Fatalf("Met() error = %v", err)
			}
			if got != tt.wantMet {
				t.Errorf("Met() = %v, want %v", got, tt.wantMet)
			}
		})
	}
}

func TestNot(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		inner   extensions.Condition
		wantMet bool
	}{
		{"not met -> true", unmet("a"), true},
		{"not unmet -> false", met("a"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Not(tt.inner)
			got, err := c.Met(ctx)
			if err != nil {
				t.Fatalf("Met() error = %v", err)
			}
			if got != tt.wantMet {
				t.Errorf("Met() = %v, want %v", got, tt.wantMet)
			}
		})
	}
}

func TestResource_AlwaysMet(t *testing.T) {
	ctx := context.Background()
	c := Resource("package:nginx")
	got, err := c.Met(ctx)
	if err != nil {
		t.Fatalf("Met() error = %v", err)
	}
	if !got {
		t.Error("Resource condition should always be met at runtime")
	}
}

func TestResource_ID(t *testing.T) {
	c := Resource("file:/etc/config")
	if c.ID() != "file:/etc/config" {
		t.Errorf("ID() = %q", c.ID())
	}
}

func TestResourceIDs(t *testing.T) {
	tests := []struct {
		name string
		cond extensions.Condition
		want []string
	}{
		{"single resource", Resource("package:nginx"), []string{"package:nginx"}},
		{"all with resources", All(Resource("a"), met("b"), Resource("c")), []string{"a", "c"}},
		{"nested all", All(Resource("a"), All(Resource("b"), met("c"))), []string{"a", "b"}},
		{"any with resources", Any(Resource("a"), Resource("b")), []string{"a", "b"}},
		{"not with resource", Not(Resource("a")), []string{"a"}},
		{"no resources", All(met("a"), met("b")), nil},
		{"nil", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResourceIDs(tt.cond)
			if len(got) != len(tt.want) {
				t.Fatalf("ResourceIDs() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ResourceIDs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestStripResources(t *testing.T) {
	tests := []struct {
		name    string
		cond    extensions.Condition
		wantNil bool
		wantStr string
	}{
		{"pure resource -> nil", Resource("a"), true, ""},
		{"all resources -> nil", All(Resource("a"), Resource("b")), true, ""},
		{"mixed -> keeps non-resource", All(Resource("a"), met("b")), false, "b"},
		{"no resources -> unchanged", met("b"), false, "b"},
		{"nil -> nil", nil, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripResources(tt.cond)
			if tt.wantNil {
				if got != nil {
					t.Errorf("StripResources() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("StripResources() = nil, want non-nil")
			}
			if got.String() != tt.wantStr {
				t.Errorf("StripResources().String() = %q, want %q", got.String(), tt.wantStr)
			}
		})
	}
}

func TestAll_String(t *testing.T) {
	c := All(met("a"), met("b"))
	want := "all(a, b)"
	if got := c.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestAny_String(t *testing.T) {
	c := Any(met("a"), met("b"))
	want := "any(a, b)"
	if got := c.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestNot_String(t *testing.T) {
	c := Not(met("a"))
	want := "not(a)"
	if got := c.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestResource_String(t *testing.T) {
	c := Resource("package:nginx")
	want := "resource(package:nginx)"
	if got := c.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
