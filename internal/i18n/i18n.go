// Package i18n translates what the bridge prints in the terminal: log lines and flag help.
// The office page translates itself (web/i18n.js); the mod carries its own few strings.
//
// Each language is one file, locales/<lang>.json, with the same keys as en.json.
// A message may hold {name} placeholders, filled from T's name/value pairs.
package i18n

import (
	"embed"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
)

//go:embed locales/*.json
var files embed.FS

// Langs are the supported languages; English comes first and is the fallback.
var Langs = []string{"en", "zh-CN", "zh-TW", "ja", "ko", "es", "fr", "de", "pt-BR", "ru"}

var (
	mu       sync.Mutex
	current  = "en"
	catalogs = map[string]map[string]string{}
)

// Match maps a language tag ("zh_CN.UTF-8", "zh-Hant-HK", "pt", "en-US") to a supported
// language, or "" when none fits.
func Match(tag string) string {
	tag = strings.TrimSpace(tag)
	if i := strings.IndexAny(tag, ".@"); i >= 0 { // drop ".UTF-8" and "@euro"
		tag = tag[:i]
	}
	parts := strings.Split(strings.ToLower(strings.ReplaceAll(tag, "_", "-")), "-")
	switch parts[0] {
	case "", "c", "posix":
		return ""
	case "zh":
		for _, p := range parts[1:] {
			if p == "hant" || p == "tw" || p == "hk" || p == "mo" {
				return "zh-TW"
			}
		}
		return "zh-CN"
	case "pt":
		return "pt-BR"
	}
	for _, l := range Langs {
		if strings.ToLower(l) == parts[0] {
			return l
		}
	}
	return ""
}

// Detect picks the language from the environment: SYMPHONY_LANG first, then the POSIX locale,
// where the first of LC_ALL, LC_MESSAGES and LANG that is set decides. English otherwise.
func Detect(getenv func(string) string) string {
	if l := Match(getenv("SYMPHONY_LANG")); l != "" {
		return l
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := getenv(k); v != "" {
			if l := Match(v); l != "" {
				return l
			}
			break
		}
	}
	return "en"
}

// SetLang sets the language T uses; an unsupported one means English.
func SetLang(lang string) {
	if lang = Match(lang); lang == "" {
		lang = "en"
	}
	mu.Lock()
	current = lang
	mu.Unlock()
}

// Lang is the language T uses.
func Lang() string {
	mu.Lock()
	defer mu.Unlock()
	return current
}

// T returns the message for key in the current language, falling back to English and then to
// the key itself, with each {name} replaced: T("log.terminal", "terminal", "tmux").
func T(key string, args ...string) string {
	params := map[string]string{}
	for i := 0; i+1 < len(args); i += 2 {
		params[args[i]] = args[i+1]
	}
	return TMap(key, params)
}

// Has reports whether English has a message for key, which every language then has too.
func Has(key string) bool {
	mu.Lock()
	defer mu.Unlock()
	_, ok := catalog("en")[key]
	return ok
}

// TMap is T with the placeholder values in a map.
func TMap(key string, params map[string]string) string {
	return TLang(Lang(), key, params)
}

// TLang is TMap in the given language rather than the bridge's own (a task's kickoff speaks the page's language);
// an unsupported language means English.
func TLang(lang, key string, params map[string]string) string {
	if lang = Match(lang); lang == "" {
		lang = "en"
	}
	mu.Lock()
	msg, ok := catalog(lang)[key]
	if !ok {
		msg, ok = catalog("en")[key]
	}
	mu.Unlock()
	if !ok {
		msg = key
	}
	// one pass, so a value that holds {braces} (a user's text, say) is never filled in again
	return placeholder.ReplaceAllStringFunc(msg, func(m string) string {
		if v, ok := params[m[1:len(m)-1]]; ok {
			return v
		}
		return m
	})
}

var placeholder = regexp.MustCompile(`\{(\w+)\}`)

// catalog loads a language's messages once; the caller holds mu.
func catalog(lang string) map[string]string {
	if c, ok := catalogs[lang]; ok {
		return c
	}
	c := map[string]string{}
	if data, err := files.ReadFile("locales/" + lang + ".json"); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	catalogs[lang] = c
	return c
}
