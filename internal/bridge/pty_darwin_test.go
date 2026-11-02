//go:build darwin

package bridge

import (
	"slices"
	"testing"
)

// TestParsePSEnv: ps -E -o pid= -o command= gives the id, the command and the environment a word each; a process is
// found by one of those words being exactly key=value.
func TestParsePSEnv(t *testing.T) {
	out := "  123 /bin/sh -c sleep 1000 PATH=/usr/bin SYMPHONY_LAUNCH=L7 HOME=/Users/me\n" +
		"  124 sleep 1000 SYMPHONY_LAUNCH=L70\n" +
		"  125 /usr/bin/true SYMPHONY_LAUNCH=L7\n" +
		"junk\n"
	if got := parsePSEnv(out, "SYMPHONY_LAUNCH=L7"); !slices.Equal(got, []int{123, 125}) {
		t.Fatalf("parsePSEnv: %v", got)
	}
}
