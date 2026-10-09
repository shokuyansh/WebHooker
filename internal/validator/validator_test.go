package validator

import (
	"reflect"
	"testing"
)

func TestValidator(t *testing.T) {
	v := New()
	if v.Errors == nil || !v.Valid() {
		t.Fatal("new validator must have an initialized empty error map")
	}
	v.Check(true, "name", "ignored")
	if !v.Valid() {
		t.Fatal("successful check added an error")
	}
	v.Check(false, "name", "required")
	v.AddError("name", "replacement")
	v.AddError("payload", "invalid")
	want := map[string]string{"name": "required", "payload": "invalid"}
	if v.Valid() || !reflect.DeepEqual(v.Errors, want) {
		t.Fatalf("errors = %v; want %v", v.Errors, want)
	}
}

func TestUnique(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []string
		want   bool
	}{
		{"nil", nil, true}, {"empty", []string{}, true},
		{"distinct", []string{"a", "b"}, true}, {"duplicate", []string{"a", "b", "a"}, false},
		{"case sensitive", []string{"A", "a"}, true}, {"empty duplicate", []string{"", ""}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Unique(tc.values); got != tc.want {
				t.Fatalf("Unique(%v) = %v; want %v", tc.values, got, tc.want)
			}
		})
	}
	if Unique([]int{1, 2, 1}) || !Unique([]int{1, 2, 3}) {
		t.Fatal("integer uniqueness is incorrect")
	}
}
