package db_test

import (
	"context"
	"strings"
	"testing"
	"unicode"
)

// The database's idea of display text and the application's have to be
// the same idea, because a value crosses between them twice on an
// ordinary day: out through the CSV export, and back in through the
// import, with strings.TrimSpace applied to every cell on the way.
//
// Where they disagreed, the round trip was not the identity. A name
// with a leading U+00A0 — which arrives from a spreadsheet or a paste
// more or less constantly at a school that types Chinese — was stored
// as given, exported as given, and re-imported as a *different* name,
// with nothing on screen to show what changed. A name that was only
// U+00A0 was worse: it was accepted, and then made the administrator's
// own export unimportable, with no visible character to search for.

func singleLine(t *testing.T, s string) bool {
	t.Helper()

	pool, _ := fresh(t)

	var ok bool
	if err := pool.QueryRow(context.Background(),
		`SELECT is_single_line($1)`, s).Scan(&ok); err != nil {
		t.Fatalf("is_single_line(%q): %v", s, err)
	}

	return ok
}

func TestTheDatabaseAndGoAgreeOnWhitespace(t *testing.T) {
	t.Parallel()

	pool, _ := fresh(t)
	ctx := context.Background()

	// Every codepoint in the two planes where whitespace lives, so the
	// agreement is checked rather than sampled at the characters
	// somebody thought of.
	for i := range 0x3100 {
		r := rune(i)

		if !unicode.IsPrint(r) && !unicode.IsSpace(r) {
			// Control characters are rejected by both sides for a
			// different reason, which the next test covers.
			continue
		}

		// "x" on both sides so the only thing under test is whether
		// this rune counts as an edge to trim.
		padded := string(r) + "x" + string(r)

		var accepted bool
		if err := pool.QueryRow(ctx, `SELECT is_single_line($1)`, padded).Scan(&accepted); err != nil {
			t.Fatalf("is_single_line(U+%04X): %v", r, err)
		}

		trimmed := strings.TrimSpace(padded) != padded

		// A rune Go trims must be one the database refuses at an edge,
		// or the export/import round trip silently rewrites the value.
		// A rune Go does not trim must be one the database accepts, or
		// a name a person can legitimately type is refused.
		if unicode.IsSpace(r) && accepted {
			t.Errorf("U+%04X: Go trims it, the database accepts it at an edge; "+
				"a re-imported export would change the value", r)
		}

		if trimmed != unicode.IsSpace(r) {
			t.Errorf("U+%04X: TrimSpace disagrees with unicode.IsSpace", r)
		}
	}
}

func TestInvisibleCharactersCannotHideInsideAName(t *testing.T) {
	t.Parallel()

	// Not whitespace to either side, and not caught by trimming
	// anything, because they sit in the middle. Two names differing by
	// one of these are two rows nobody can tell apart, and the bidi
	// overrides reorder the text around them in every list.
	for name, s := range map[string]string{
		"zero-width space":     "Wei\u200bChen",
		"byte order mark":      "\ufeffWei",
		"left-to-right mark":   "Wei\u200eChen",
		"left-to-right over":   "\u202dWei",
		"right-to-left over":   "Wei\u202eChen",
		"word joiner":          "Wei\u2060Chen",
		"soft hyphen":          "Wei\u00adChen",
		"first strong isolate": "Wei\u2068Chen",
		// Zl and Zp, so [[:cntrl:]] does not reach them, and they are
		// whitespace so trimming only covered the edges. U+2028 is a
		// forced line break in CSS text: this name is two lines tall
		// in every table cell that shows it.
		"line separator":      "Chess\u2028Club",
		"paragraph separator": "Chess\u2029Club",
	} {
		if singleLine(t, s) {
			t.Errorf("%s accepted in %q", name, s)
		}
	}

	// A name that renders as nothing at all. Each of these is a
	// legal, non-whitespace, non-control character that occupies no
	// space on screen, so a row made of them appeared blank in every
	// list, could not be searched for, and could not be told from the
	// row above it. The tag block is the sharp one: it is a complete
	// invisible alphabet, so a name can carry a second name inside it.
	for name, s := range map[string]string{
		"Hangul filler":              "\u3164",
		"Hangul choseong filler":     "\u115f",
		"Hangul jungseong filler":    "\u1160",
		"halfwidth Hangul filler":    "\uffa0",
		"Mongolian vowel separator":  "\u180e",
		"interlinear annotation":     "\ufff9",
		"a tag character":            "\U000E0041",
		"a whole word in tag":        "\U000E0068\U000E0069",
		"a tag character in a name":  "Wei\U000E0041Chen",
		"the tag block's last entry": "\U000E007F",
	} {
		if singleLine(t, s) {
			t.Errorf("%s accepted in %q; that name is invisible", name, s)
		}
	}

	// The counter-case, and the reason this is a list rather than a
	// category: the zero-width joiner builds emoji sequences and is how
	// several scripts are written correctly. Refusing it would reject
	// real names to make a point about fake ones.
	for name, s := range map[string]string{
		"emoji ZWJ sequence": "\U0001F468\u200d\U0001F3EB Wei",
		"Persian ZWNJ":       "\u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645",
		"CJK":                "\u5f20\u4e09",
		"combining marks":    "José",
	} {
		if !singleLine(t, s) {
			t.Errorf("%s rejected in %q; it is a name somebody has", name, s)
		}
	}
}

func TestDisplayTextIsBoundedSoOneRowCannotHideTheRest(t *testing.T) {
	t.Parallel()

	// Unbounded, a single name is a page 100,000 pixels wide with every
	// other row pushed off the side of it. The bound is far past any
	// real course, teacher, room or term.
	if !singleLine(t, strings.Repeat("X", 200)) {
		t.Error("200 characters rejected; the bound is meant to be generous")
	}

	if singleLine(t, strings.Repeat("X", 201)) {
		t.Error("201 characters accepted; the bound does not bind")
	}

	// Counted in characters, not bytes, or a Chinese name would get a
	// third of the room an English one does.
	if !singleLine(t, strings.Repeat("\u5f20", 200)) {
		t.Error("200 Chinese characters rejected; the bound is counting bytes")
	}
}
