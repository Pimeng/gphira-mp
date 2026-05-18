package l10n

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
)

//go:embed lang/zh-CN.ftl
var defaultZhCN string

//go:embed lang/en-US.ftl
var defaultEnUS string

// Language represents a language instance for localization.
type Language struct {
	lang string
	dict map[string]string
}

// Lang returns the language code.
func (l *Language) Lang() string {
	if l == nil {
		return "zh-CN"
	}
	return l.lang
}

// New creates a new Language instance.
// It loads embedded defaults first, then overlays any external
// file at l10n/{lang}.ftl. This allows partial overrides:
// the external file only needs to contain the keys you want to change.
func New(lang string) *Language {
	return &Language{
		lang: lang,
		dict: loadTranslations(lang),
	}
}

func loadTranslations(lang string) map[string]string {
	// 1. Start with embedded defaults
	var base string
	switch lang {
	case "en-US":
		base = defaultEnUS
	case "zh-CN", "":
		base = defaultZhCN
	default:
		base = defaultZhCN
	}
	dict := parseDict(base)

	// 2. Overlay external file (if present) on top of defaults
	cwd, _ := os.Getwd()
	for dir := cwd; dir != ""; dir = filepath.Dir(dir) {
		extPath := filepath.Join(dir, "l10n", lang+".ftl")
		if data, err := os.ReadFile(extPath); err == nil {
			for k, v := range parseDict(string(data)) {
				dict[k] = v
			}
			break
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}

	return dict
}

// parseDict parses a simple key=value text format.
// Empty lines and lines starting with '#' are ignored.
func parseDict(content string) map[string]string {
	dict := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		dict[key] = val
	}
	return dict
}

// Format returns the translated string for the given key,
// performing simple variable replacement using args.
// If the key is not found, the key itself is returned.
func (l *Language) Format(key string, args map[string]string) string {
	if l == nil {
		return key
	}
	tmpl, ok := l.dict[key]
	if !ok {
		return key
	}
	if len(args) == 0 {
		return tmpl
	}
	result := tmpl
	for k, v := range args {
		result = strings.ReplaceAll(result, "{ $"+k+" }", v)
		result = strings.ReplaceAll(result, "{$"+k+"}", v)
	}
	return result
}

// TL is a shorthand for translating a key with the given language and arguments.
func TL(lang *Language, key string, args map[string]string) string {
	if lang == nil {
		return key
	}
	return lang.Format(key, args)
}
