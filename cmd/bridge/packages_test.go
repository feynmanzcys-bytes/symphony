package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// runPackages runs .github/packages.sh, which writes the Homebrew formula and the Scoop manifest of a release, and
// returns what it printed.
func runPackages(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the release scripts run on Linux and macOS")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("needs bash")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", ".github", "packages.sh"))
	if err != nil {
		t.Fatal(err)
	}
	return exec.Command(bash, append([]string{script}, args...)...).CombinedOutput()
}

// TestPackages checks what .github/packages.sh writes from made-up checksums (not in release.sh's order, one hashed in
// binary mode, one in uppercase, one for an archive the packages don't use) against testdata/packages. After changing
// the script on purpose, write those again with
//
//	.github/packages.sh v1.2.3 cmd/bridge/testdata/packages/checksums.txt cmd/bridge/testdata/packages
func TestPackages(t *testing.T) {
	data := filepath.Join("testdata", "packages")
	out := t.TempDir()
	if b, err := runPackages(t, "v1.2.3", filepath.Join(data, "checksums.txt"), out); err != nil {
		t.Fatalf("packages.sh: %v\n%s", err, b)
	}
	var wrote []string
	err := filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(out, path)
			wrote = append(wrote, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	files := []string{"Formula/symphony.rb", "bucket/symphony.json"}
	if !slices.Equal(wrote, files) {
		t.Fatalf("packages.sh wrote %q, want %q", wrote, files)
	}
	for _, name := range files {
		got, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(data, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s isn't testdata/packages/%s; packages.sh wrote:\n%s", name, name, got)
		}
	}

	// Scoop reads it as JSON: every archive where Scoop looks for it, and the autoupdate's checksums.txt
	raw, err := os.ReadFile(filepath.Join(out, "bucket", "symphony.json"))
	if err != nil {
		t.Fatal(err)
	}
	type archive struct{ URL, Hash string }
	var m struct {
		Version      string
		Bin          string
		Architecture map[string]archive
		Autoupdate   struct {
			Architecture map[string]archive
			Hash         struct{ URL string }
		}
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("the manifest isn't JSON: %v", err)
	}
	download := "https://github.com/feynmanzcys-bytes/symphony/releases/download/"
	wantArch := map[string]archive{
		"64bit": {download + "v1.2.3/symphony_windows_amd64.zip", strings.Repeat("3a", 32)},
		"arm64": {download + "v1.2.3/symphony_windows_arm64.zip", strings.Repeat("3b", 32)},
	}
	wantAuto := map[string]archive{
		"64bit": {URL: download + "v$version/symphony_windows_amd64.zip"},
		"arm64": {URL: download + "v$version/symphony_windows_arm64.zip"},
	}
	switch {
	case m.Version != "1.2.3", m.Bin != "symphony.exe":
		t.Errorf("version %q, bin %q", m.Version, m.Bin)
	case !maps.Equal(m.Architecture, wantArch):
		t.Errorf("architecture %v, want %v", m.Architecture, wantArch)
	case !maps.Equal(m.Autoupdate.Architecture, wantAuto), m.Autoupdate.Hash.URL != "$baseurl/checksums.txt":
		t.Errorf("autoupdate %+v", m.Autoupdate)
	}
}

// TestPackagesRefuses checks that packages.sh writes nothing for a tag that isn't a release's, or from checksums that
// don't give every archive exactly one SHA-256.
func TestPackagesRefuses(t *testing.T) {
	good, err := os.ReadFile(filepath.Join("testdata", "packages", "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	sums := string(good)
	without := func(archive string) string {
		var keep []string
		for _, line := range strings.SplitAfter(sums, "\n") {
			if !strings.HasSuffix(strings.TrimSpace(line), archive) {
				keep = append(keep, line)
			}
		}
		return strings.Join(keep, "")
	}
	for _, c := range []struct{ name, tag, sums, says string }{
		{"a pre-release", "v1.2.3-rc.1", sums, "not a release tag"},
		{"no v", "1.2.3", sums, "not a release tag"},
		{"an archive missing", "v1.2.3", without("symphony_windows_arm64.zip"), "symphony_windows_arm64.zip"},
		{"an archive twice", "v1.2.3", sums + strings.Repeat("0", 64) + "  symphony_linux_amd64.tar.gz\n", "symphony_linux_amd64.tar.gz"},
		{"not a SHA-256", "v1.2.3", strings.Replace(sums, strings.Repeat("21", 32), strings.Repeat("21", 20), 1), "symphony_darwin_amd64.tar.gz"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "checksums.txt")
			if err := os.WriteFile(file, []byte(c.sums), 0o644); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "out")
			b, err := runPackages(t, c.tag, file, out)
			if err == nil {
				t.Fatalf("packages.sh succeeded:\n%s", b)
			}
			if !strings.Contains(string(b), c.says) {
				t.Errorf("packages.sh said %q, want it to name %q", b, c.says)
			}
			if _, err := os.Stat(out); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("packages.sh made %s anyway", out)
			}
		})
	}
}
