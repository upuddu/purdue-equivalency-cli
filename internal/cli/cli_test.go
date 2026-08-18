package cli

import (
	"slices"
	"testing"
)

func TestPartitionArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantFlags    []string
		wantOperands []string
	}{
		{
			name:         "flags after operands",
			args:         []string{"MA", "42500", "--json"},
			wantFlags:    []string{"--json"},
			wantOperands: []string{"MA", "42500"},
		},
		{
			name:         "flag value stays with its flag",
			args:         []string{"MA", "--state", "WI", "42500"},
			wantFlags:    []string{"--state", "WI"},
			wantOperands: []string{"MA", "42500"},
		},
		{
			name:         "inline flag value",
			args:         []string{"--state=WI", "MA"},
			wantFlags:    []string{"--state=WI"},
			wantOperands: []string{"MA"},
		},
		{
			name:         "everything after a double dash is an operand",
			args:         []string{"--json", "--", "--not-a-flag"},
			wantFlags:    []string{"--json"},
			wantOperands: []string{"--not-a-flag"},
		},
		{
			name:         "no arguments",
			args:         nil,
			wantFlags:    nil,
			wantOperands: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, operands := partitionArgs(tt.args)

			if !slices.Equal(flags, tt.wantFlags) {
				t.Errorf("flags = %q, want %q", flags, tt.wantFlags)
			}
			if !slices.Equal(operands, tt.wantOperands) {
				t.Errorf("operands = %q, want %q", operands, tt.wantOperands)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in    string
		limit int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly-ten", 11, "exactly-ten"},
		{"a much longer string", 10, "a much lo…"},
		{"café-au-lait", 6, "café-…"}, // counts runes, not bytes
	}

	for _, tt := range tests {
		if got := truncate(tt.in, tt.limit); got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.in, tt.limit, got, tt.want)
		}
	}
}
