package cmd

import (
	"reflect"
	"testing"
)

func TestReorderFlagsFirst(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "flag before positional (already fine)",
			args: []string{"-o", "-", "stage/auth-01"},
			want: []string{"-o", "-", "stage/auth-01"},
		},
		{
			name: "flag after positional",
			args: []string{"stage/auth-01", "-o", "-"},
			want: []string{"-o", "-", "stage/auth-01"},
		},
		{
			name: "equals form",
			args: []string{"stage/auth-01", "-vault=/tmp/v.enc"},
			want: []string{"-vault=/tmp/v.enc", "stage/auth-01"},
		},
		{
			name: "no flags at all",
			args: []string{"stage/auth-01"},
			want: []string{"stage/auth-01"},
		},
		{
			name: "multiple flags interleaved",
			args: []string{"stage/auth-01", "-o", "-", "-vault", "/tmp/v.enc"},
			want: []string{"-o", "-", "-vault", "/tmp/v.enc", "stage/auth-01"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := reorderFlagsFirst(tc.args, "vault", "o")
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("reorderFlagsFirst(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
