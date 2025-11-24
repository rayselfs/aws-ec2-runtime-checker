package main

import (
	"os"
	"testing"
)

func TestIsCronMode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "cron mode enabled",
			args: []string{"cron"},
			want: true,
		},
		{
			name: "cron mode disabled - no args",
			args: []string{},
			want: false,
		},
		{
			name: "cron mode disabled - different arg",
			args: []string{"run"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original args
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()

			// Set test args
			os.Args = append([]string{"cmd"}, tt.args...)

			if got := isCronMode(); got != tt.want {
				t.Errorf("isCronMode() = %v, want %v", got, tt.want)
			}
		})
	}
}
