package l10n

import (
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/lus/fluent.go/fluent"
	"golang.org/x/text/language"
)

//go:embed lang/zh-CN.ftl
var defaultZhCN string

//go:embed lang/en-US.ftl
var defaultEnUS string

// Language represents a language instance for localization. It wraps a Fluent
// bundle assembled from embedded defaults and optional external overlay files.
type Language struct {
	lang   string
	bundle *fluent.Bundle
}

// Lang returns the language code.
func (l *Language) Lang() string {
	if l == nil {
		return "zh-CN"
	}
	return l.lang
}

// New creates a new Language instance.
// It loads embedded defaults first, then overlays any external file at
// l10n/{lang}.ftl. This allows partial overrides: the external file only
// needs to contain the keys you want to change.
func New(lang string) *Language {
	return &Language{
		lang:   lang,
		bundle: buildBundle(lang),
	}
}

func buildBundle(lang string) *fluent.Bundle {
	var base string
	var tag language.Tag
	switch lang {
	case "en-US":
		base = defaultEnUS
		tag = language.AmericanEnglish
	case "zh-CN", "":
		base = defaultZhCN
		tag = language.SimplifiedChinese
	default:
		base = defaultZhCN
		tag = language.SimplifiedChinese
	}

	bundle := fluent.NewBundle(tag)
	if base != "" {
		res, errs := fluent.NewResource(base)
		if len(errs) > 0 {
			slog.Warn("l10n: embedded resource parse errors", "lang", lang, "count", len(errs))
		}
		if addErrs := bundle.AddResource(res); len(addErrs) > 0 {
			slog.Warn("l10n: embedded resource add errors", "lang", lang, "count", len(addErrs))
		}
	}

	if overlay := readExternalOverlay(lang); overlay != "" {
		res, errs := fluent.NewResource(overlay)
		if len(errs) > 0 {
			slog.Warn("l10n: external resource parse errors", "lang", lang, "count", len(errs))
		}
		bundle.AddResourceOverriding(res)
	}

	return bundle
}

// readExternalOverlay walks up from the current working directory looking for
// l10n/{lang}.ftl. Returns the file content if found, "" otherwise.
func readExternalOverlay(lang string) string {
	cwd, _ := os.Getwd()
	for dir := cwd; dir != ""; dir = filepath.Dir(dir) {
		extPath := filepath.Join(dir, "l10n", lang+".ftl")
		if data, err := os.ReadFile(extPath); err == nil {
			return string(data)
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return ""
}

// Format returns the translated string for the given key, performing variable
// substitution via the underlying Fluent bundle. If the key is missing or
// formatting fails, the key itself is returned as a fallback.
func (l *Language) Format(key string, args map[string]string) string {
	if l == nil || l.bundle == nil {
		return key
	}
	if !l.bundle.HasMessage(key) {
		slog.Warn("l10n: missing key", "lang", l.lang, "key", key)
		return key
	}

	contexts := buildFormatContexts(args)
	out, ferrs, err := l.bundle.FormatMessage(key, contexts...)
	if err != nil {
		slog.Warn("l10n: format failed", "lang", l.lang, "key", key, "err", err)
		return key
	}
	if len(ferrs) > 0 {
		slog.Warn("l10n: format had errors", "lang", l.lang, "key", key, "count", len(ferrs))
	}
	return out
}

func buildFormatContexts(args map[string]string) []*fluent.FormatContext {
	if len(args) == 0 {
		return nil
	}
	ctxs := make([]*fluent.FormatContext, 0, len(args))
	for k, v := range args {
		ctxs = append(ctxs, fluent.WithVariable(k, v))
	}
	return ctxs
}

// TL is a shorthand for translating a key with the given language and arguments.
func TL(lang *Language, key string, args map[string]string) string {
	if lang == nil {
		return key
	}
	return lang.Format(key, args)
}
