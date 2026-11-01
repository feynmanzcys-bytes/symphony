package bridge

import "testing"

// TestParseStat counts the fields of /proc/<pid>/stat from the name's last ")": a name may hold both.
func TestParseStat(t *testing.T) {
	stat := "4242 (a) (b c) S 17 4242 4242 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 1 0 987654 1000 100 18446744073709551615"
	p, ok := parseStat(stat)
	if want := (procInfo{pid: 4242, ppid: 17, pgid: 4242, start: "987654"}); !ok || p != want {
		t.Fatalf("got %+v %v, want %+v", p, ok, want)
	}
	for _, bad := range []string{"", "4242 (x", "4242 (x) S 1 2", "x (y) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20"} {
		if _, ok := parseStat(bad); ok {
			t.Errorf("%q should not parse", bad)
		}
	}
}
