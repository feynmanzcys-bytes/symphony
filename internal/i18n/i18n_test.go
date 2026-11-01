package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	for tag, want := range map[string]string{
		"en": "en", "en-US": "en", "en_GB.UTF-8": "en",
		"zh": "zh-CN", "zh_CN.UTF-8": "zh-CN", "zh-Hans": "zh-CN", "zh-SG": "zh-CN",
		"zh-TW": "zh-TW", "zh_HK.UTF-8": "zh-TW", "zh-Hant-MO": "zh-TW", "zh-hant": "zh-TW",
		"ja_JP.UTF-8": "ja", "ko-KR": "ko", "es-419": "es", "fr_CA": "fr", "de_DE@euro": "de",
		"pt": "pt-BR", "pt-PT": "pt-BR", "pt_BR.UTF-8": "pt-BR", "ru_RU.KOI8-R": "ru",
		"": "", "C": "", "POSIX": "", "C.UTF-8": "", "it_IT": "", "xx": "",
	} {
		if got := Match(tag); got != want {
			t.Errorf("Match(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestDetect(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	for _, c := range []struct {
		vars map[string]string
		want string
	}{
		{map[string]string{}, "en"},
		{map[string]string{"LANG": "ja_JP.UTF-8"}, "ja"},
		{map[string]string{"LANG": "ja_JP.UTF-8", "LC_MESSAGES": "de_DE.UTF-8"}, "de"},
		{map[string]string{"LANG": "ja_JP.UTF-8", "LC_ALL": "ko_KR.UTF-8"}, "ko"},
		// POSIX: the first variable that is set decides, even when it is "C"
		{map[string]string{"LANG": "ja_JP.UTF-8", "LC_ALL": "C"}, "en"},
		{map[string]string{"LANG": "it_IT.UTF-8"}, "en"},
		{map[string]string{"LANG": "ja_JP.UTF-8", "SYMPHONY_LANG": "zh-TW"}, "zh-TW"},
		{map[string]string{"LANG": "ja_JP.UTF-8", "SYMPHONY_LANG": "xx"}, "ja"},
	} {
		if got := Detect(env(c.vars)); got != c.want {
			t.Errorf("Detect(%v) = %q, want %q", c.vars, got, c.want)
		}
	}
}

func TestT(t *testing.T) {
	t.Cleanup(func() { SetLang("en") })
	SetLang("zh_CN.UTF-8")
	if Lang() != "zh-CN" {
		t.Fatalf("SetLang should match the tag, got %q", Lang())
	}
	if got := T("log.launch_off_claude", "claude", "cc"); got != "页面启动会话关闭：找不到 cc" {
		t.Fatalf("zh-CN message: %q", got)
	}
	SetLang("en")
	if got := T("log.mod_installed", "id", "symphony-bridge@symphony", "version", "0.3.0"); got != "mod: symphony-bridge@symphony 0.3.0 is installed" {
		t.Fatalf("en message: %q", got)
	}
	if got := T("no.such.key"); got != "no.such.key" {
		t.Fatalf("a missing key should come back as itself, got %q", got)
	}
	SetLang("xx")
	if Lang() != "en" {
		t.Fatalf("an unsupported language should mean English, got %q", Lang())
	}
}

// TestTLang: a message in a language other than the bridge's own, English for an unsupported one, and values filled
// in once (a value holding {braces} stays as it is).
func TestTLang(t *testing.T) {
	t.Cleanup(func() { SetLang("en") })
	SetLang("ja")
	line := map[string]string{"name": "bob-T1", "dir": "/w/{dir}", "branch": "symphony/T1/bob", "worktree": "/w/.symphony/worktrees/bob-T1"}
	if got := TLang("zh_CN.UTF-8", "task.kickoff.teammate", line); got != "- bob-T1：目录 /w/{dir}，分支 symphony/T1/bob，工作树 /w/.symphony/worktrees/bob-T1" {
		t.Fatalf("zh-CN message: %q", got)
	}
	if got := TLang("xx", "task.kickoff.teammate", line); got != "- bob-T1: folder /w/{dir}, branch symphony/T1/bob, worktree /w/.symphony/worktrees/bob-T1" {
		t.Fatalf("an unsupported language should mean English: %q", got)
	}
	if Lang() != "ja" {
		t.Fatalf("TLang must leave the bridge's language alone, got %q", Lang())
	}
}

// TestCatalogs checks every language against English, for the bridge's catalogs and the page's:
// the same keys, and the same {placeholders} in each message, so no translation drops or invents a value.
func TestCatalogs(t *testing.T) {
	checkCatalogs(t, func(lang string) ([]byte, error) { return files.ReadFile("locales/" + lang + ".json") })
}

func TestWebCatalogs(t *testing.T) {
	checkCatalogs(t, func(lang string) ([]byte, error) { return os.ReadFile("../../web/locales/" + lang + ".json") })
}

// TestKeysInUse checks that every key the Go code asks for with T("...") exists in English.
func TestKeysInUse(t *testing.T) {
	data, err := files.ReadFile("locales/en.json")
	if err != nil {
		t.Fatal(err)
	}
	var en map[string]string
	if err := json.Unmarshal(data, &en); err != nil {
		t.Fatal(err)
	}
	call := regexp.MustCompile(`i18n\.T(?:Map)?\("([^"]+)"\s*[,)]`) // literal keys only; "error."+code is dynamic
	inLang := regexp.MustCompile(`i18n\.TLang\([^,()]+,\s*"([^"]+)"\s*,`)
	used := 0
	for _, dir := range []string{"../../cmd", "../../internal"} {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, re := range []*regexp.Regexp{call, inLang} {
				for _, m := range re.FindAllStringSubmatch(string(src), -1) {
					used++
					if _, ok := en[m[1]]; !ok {
						t.Errorf("%s asks for %q, which en.json lacks", path, m[1])
					}
				}
			}
			return nil
		})
	}
	if used < 10 {
		t.Fatalf("found only %d T(...) calls; is the scan looking in the right place?", used)
	}
}

// TestWebKeysInUse checks that every key the pages name exists in the page's English catalog: string
// literals in a catalog namespace, and the data-i18n attributes. Keys built at run time ('state.' + state)
// are left to the end-to-end test, which records every key a page misses.
func TestWebKeysInUse(t *testing.T) {
	data, err := os.ReadFile("../../web/locales/en.json")
	if err != nil {
		t.Fatal(err)
	}
	var en map[string]string
	if err := json.Unmarshal(data, &en); err != nil {
		t.Fatal(err)
	}
	literal := regexp.MustCompile("['\"`]((?:common|office|hud|pixels|room|state|idle|search|stats|log|asks|msg|card|chat|launch|option|approve|error|task|hq)\\.[A-Za-z0-9_.]*[A-Za-z0-9_])['\"`]")
	attr := regexp.MustCompile(`data-i18n(?:-placeholder|-title)?="([^"]+)"`)
	used := 0
	files := []string{"approve.html", "asks.js", "i18n.js"}
	for _, pattern := range []string{"symphony.js", "ui/*.js", "themes/*/index.html", "themes/*/*.js"} {
		hits, _ := filepath.Glob("../../web/" + pattern)
		for _, h := range hits {
			files = append(files, strings.TrimPrefix(filepath.ToSlash(h), "../../web/"))
		}
	}
	// a theme's own catalog adds to the shared one for the theme's files
	themeKeys := map[string]map[string]string{}
	for _, name := range files {
		src, err := os.ReadFile("../../web/" + name)
		if err != nil {
			t.Fatal(err)
		}
		known := en
		if strings.HasPrefix(name, "themes/") {
			theme := strings.Split(name, "/")[1]
			if _, ok := themeKeys[theme]; !ok {
				own := map[string]string{}
				if data, err := os.ReadFile("../../web/themes/" + theme + "/locales/en.json"); err == nil {
					_ = json.Unmarshal(data, &own)
				}
				themeKeys[theme] = own
			}
			known = map[string]string{}
			for k, v := range en {
				known[k] = v
			}
			for k, v := range themeKeys[theme] {
				known[k] = v
			}
		}
		for _, re := range []*regexp.Regexp{literal, attr} {
			for _, m := range re.FindAllStringSubmatch(string(src), -1) {
				used++
				if _, ok := known[m[1]]; !ok {
					t.Errorf("%s names %q, which web/locales/en.json lacks", name, m[1])
				}
			}
		}
	}
	if used < 50 {
		t.Fatalf("found only %d keys in the pages; is the scan looking in the right place?", used)
	}
}

// TestThemeCatalogs checks every theme's own catalogs (web/themes/<name>/locales/<lang>.json) the way the shared ones
// are checked: a theme with an en.json has every language, with the same keys and placeholders. A theme without
// catalogs is fine.
func TestThemeCatalogs(t *testing.T) {
	dirs, _ := filepath.Glob("../../web/themes/*/locales")
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "en.json")); err != nil {
			continue
		}
		t.Run(filepath.Base(filepath.Dir(dir)), func(t *testing.T) {
			checkCatalogs(t, func(lang string) ([]byte, error) { return os.ReadFile(filepath.Join(dir, lang+".json")) })
		})
	}
}

func checkCatalogs(t *testing.T, read func(lang string) ([]byte, error)) {
	t.Helper()
	load := func(lang string) map[string]string {
		data, err := read(lang)
		if err != nil {
			t.Fatalf("%s: %v", lang, err)
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%s: not a JSON object of strings: %v", lang, err)
		}
		return m
	}
	params := func(s string) []string {
		var names []string
		for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
			if !slices.Contains(names, m[1]) {
				names = append(names, m[1])
			}
		}
		slices.Sort(names)
		return names
	}
	en := load("en")
	for _, lang := range Langs[1:] {
		other := load(lang)
		for key, msg := range en {
			got, ok := other[key]
			switch {
			case !ok:
				t.Errorf("%s lacks %q", lang, key)
			case strings.TrimSpace(got) == "" && strings.TrimSpace(msg) != "":
				t.Errorf("%s: %q is empty", lang, key)
			case !slices.Equal(params(got), params(msg)):
				t.Errorf("%s: %q has placeholders %v, English has %v", lang, key, params(got), params(msg))
			}
		}
		for key := range other {
			if _, ok := en[key]; !ok {
				t.Errorf("%s has %q, which English lacks", lang, key)
			}
		}
	}
}

// TestErrorCodesTranslated checks that every error code the bridge makes with newError("...") has an
// error.<code> entry in the pages' English catalog, so the page never shows a raw code.
func TestErrorCodesTranslated(t *testing.T) {
	data, err := os.ReadFile("../../web/locales/en.json")
	if err != nil {
		t.Fatal(err)
	}
	var en map[string]string
	if err := json.Unmarshal(data, &en); err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`newError\("([a-z_]+)"`)
	found := 0
	_ = filepath.WalkDir("../../internal/bridge", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range code.FindAllStringSubmatch(string(src), -1) {
			found++
			if _, ok := en["error."+m[1]]; !ok {
				t.Errorf("%s makes the error code %q, which web/locales/en.json lacks (error.%s)", path, m[1], m[1])
			}
		}
		return nil
	})
	if found < 20 {
		t.Fatalf("found only %d newError calls; is the scan looking in the right place?", found)
	}
}
