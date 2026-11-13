package i18n

import (
	"testing"
	"testing/fstest"

	"github.com/BurntSushi/toml"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// TestSysLocaleFilesHaveSameKeys keeps the key sets of the kernel's two language files equal.
//
// At runtime go-i18n falls back to the default language (en) for missing entries. That is a feature, but it also
// means an entry missing from sys.zh-CN.toml shows no symptom: the UI just has one English sentence among the
// Chinese, and nobody reports it as a bug. The registry-driven tests (cmd/admin) cover only keys that can be
// enumerated: codes, groups, resources and audit actions; keys referenced only as literals, such as error messages
// and login text, rely on this test alone. It checks only sys.*.toml: the kernel guarantees only that kernel entries
// match between English and Chinese; whether app.*.toml is fully translated is the derived repo's own concern.
func TestSysLocaleFilesHaveSameKeys(t *testing.T) {
	en := messageIDs(t, "sys.en.toml")
	zh := messageIDs(t, "sys.zh-CN.toml")

	for id := range en {
		if !zh[id] {
			t.Errorf("sys.zh-CN.toml lacks entry [%s] (present in sys.en.toml)", id)
		}
	}
	for id := range zh {
		if !en[id] {
			t.Errorf("sys.en.toml lacks entry [%s] (present in sys.zh-CN.toml)", id)
		}
	}
}

// TestBundleLoadsAllLocaleFiles runs the language-file loading that startup does: a broken file panics and fails
// here, not after deployment. It hard-codes no file name or entry: for each file it takes the file's own first entry
// and asserts it resolves in that file's language, so a derived repo's partially translated file passes too.
func TestBundleLoadsAllLocaleFiles(t *testing.T) {
	b := goi18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	files := loadMessageFiles(b, localeFS)
	if len(files) == 0 {
		t.Fatal("no embedded language files; the test asserts nothing")
	}

	for _, f := range files {
		if len(f.Messages) == 0 {
			continue
		}
		loc := goi18n.NewLocalizer(b, f.Tag.String())
		if _, err := loc.Localize(&goi18n.LocalizeConfig{MessageID: f.Messages[0].ID}); err != nil {
			t.Errorf("%s: language file not loaded into the bundle: %v", f.Path, err)
		}
	}
}

// TestLoadMessageFiles_SkipsTranslateDrafts: when translation drafts translate.*.toml sit next to the real files,
// the runtime gets the real translation. Without the skip, the draft loads later and overwrites it, and this fails.
func TestLoadMessageFiles_SkipsTranslateDrafts(t *testing.T) {
	fsys := fstest.MapFS{
		"sys.zh-CN.toml":       {Data: []byte(`Greeting = "你好"`)},
		"translate.zh-CN.toml": {Data: []byte("[Greeting]\nhash = \"sha1-x\"\nother = \"hello\"\n")},
	}
	b := goi18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	loadMessageFiles(b, fsys)

	got, err := goi18n.NewLocalizer(b, "zh-CN").Localize(&goi18n.LocalizeConfig{MessageID: "Greeting"})
	if err != nil || got != "你好" {
		t.Fatalf("expected 你好, got %q (err %v)", got, err)
	}
}

// TestMustLocalizeMessage_FallsBackToEnglish builds a language missing an entry: the Localizer picks French, but the
// bundle has the message only in English. It asserts the English source comes back with no error and no panic;
// replacing MustLocalizeMessage with localizer.MustLocalizeMessage should make this test fail with a panic.
func TestMustLocalizeMessage_FallsBackToEnglish(t *testing.T) {
	b := goi18n.NewBundle(language.English)
	if err := b.AddMessages(language.English, &goi18n.Message{ID: "Greeting", Other: "hello"}); err != nil {
		t.Fatalf("add english message: %v", err)
	}
	// French is registered in the bundle (otherwise language matching falls straight back to the default language and
	// the "language present, entry missing" path is not tested), but it has no Greeting entry.
	if err := b.AddMessages(language.French, &goi18n.Message{ID: "Unrelated", Other: "bonjour"}); err != nil {
		t.Fatalf("add french message: %v", err)
	}

	loc := goi18n.NewLocalizer(b, "fr")
	got := MustLocalizeMessage(loc, &goi18n.Message{ID: "Greeting", Other: "hello"})
	if got != "hello" {
		t.Fatalf("expected english fallback %q, got %q", "hello", got)
	}
}

func messageIDs(t *testing.T, name string) map[string]bool {
	t.Helper()
	raw, err := localeFS.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	b := goi18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	f, err := b.ParseMessageFileBytes(raw, name)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	ids := make(map[string]bool, len(f.Messages))
	for _, m := range f.Messages {
		ids[m.ID] = true
	}
	if len(ids) == 0 {
		t.Fatalf("%s: no entries parsed", name)
	}
	return ids
}
