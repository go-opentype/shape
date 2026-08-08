// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"testing"

	"github.com/go-opentype/fonts/goregular"
	"github.com/go-opentype/fonts/notoemoji"
	"github.com/go-opentype/opentype"
)

func TestIsDefaultIgnorable(t *testing.T) {
	ignorable := []rune{
		0x00AD,  // SOFT HYPHEN
		0x200B,  // ZERO WIDTH SPACE
		0x200C,  // ZERO WIDTH NON-JOINER
		0x200D,  // ZERO WIDTH JOINER
		0x200E,  // LEFT-TO-RIGHT MARK
		0x2060,  // WORD JOINER
		0xFE0F,  // VARIATION SELECTOR-16
		0xFEFF,  // ZERO WIDTH NO-BREAK SPACE (BOM)
		0xE0001, // LANGUAGE TAG
	}
	for _, r := range ignorable {
		if !isDefaultIgnorable(r) {
			t.Errorf("U+%04X should be default-ignorable", r)
		}
	}

	// Prepended concatenation marks are Cf but genuinely render — they extend
	// over the digits that follow. Hiding them would erase visible text.
	for _, r := range []rune{0x0600, 0x0601, 0x0605, 0x06DD, 0x070F, 0x08E2, 0x110BD, 0x110CD} {
		if isDefaultIgnorable(r) {
			t.Errorf("U+%04X is a prepended concatenation mark and must stay visible", r)
		}
	}

	// Ordinary text, and whitespace, are never ignorable.
	for _, r := range []rune{'a', 'Z', '0', ' ', '\t', '\n', '中', 'ا', 0x1F680} {
		if isDefaultIgnorable(r) {
			t.Errorf("U+%04X should not be default-ignorable", r)
		}
	}
}

// TestShapeHidesSurvivingIgnorables is the defect this exists for: a
// default-ignorable the font cannot map shaped to .notdef, whose full advance
// was still counted. A caller measuring the run got a gap of empty space in the
// middle of a word, because the same glyph is not painted.
func TestShapeHidesSurvivingIgnorables(t *testing.T) {
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face := f.NewFace(32)

	width := func(s string) int {
		total := 0
		for _, g := range Shape(face, s, Options{}) {
			total += g.XAdvance
		}
		return total
	}
	base := width("abcd")
	for _, r := range []rune{0x2060, 0x200B, 0x200D, 0xFEFF, 0x00AD, 0xFE0F} {
		s := "ab" + string(r) + "cd"
		if got := width(s); got != base {
			t.Errorf("U+%04X added %d px; a default-ignorable must take no width", r, got-base)
		}
	}
}

func TestShapeMarksIgnorablesInvisible(t *testing.T) {
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face := f.NewFace(32)

	// U+00AD is a real glyph in this font, so it would otherwise draw a hyphen in
	// the middle of a word. Invisible is what tells a back-end not to.
	glyphs := Shape(face, "ab­cd", Options{})
	if len(glyphs) != 5 {
		t.Fatalf("got %d glyphs, want one per rune", len(glyphs))
	}
	hidden := 0
	for _, g := range glyphs {
		if g.Invisible {
			hidden++
			if g.XAdvance != 0 || g.XOffset != 0 || g.YOffset != 0 || g.YAdvance != 0 {
				t.Fatalf("a hidden glyph still carries geometry: %+v", g)
			}
		}
	}
	if hidden != 1 {
		t.Fatalf("%d glyphs hidden, want exactly the soft hyphen", hidden)
	}
	// Every visible glyph keeps its advance.
	for _, g := range glyphs {
		if !g.Invisible && g.XAdvance <= 0 {
			t.Fatalf("a visible glyph lost its advance: %+v", g)
		}
	}
}

// TestConsumedJoinerIsNotHidden checks the rule only touches SURVIVORS: when a
// font implements an emoji ZWJ sequence, the ligature swallows the joiner, so
// no glyph carries its cluster and there is nothing to hide. Hiding a consumed
// joiner's cluster would blank the composed glyph itself.
func TestConsumedJoinerIsNotHidden(t *testing.T) {
	f, err := opentype.Parse(notoemoji.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face := f.NewFace(48)

	const astronaut = "\U0001F9D1‍\U0001F680"
	glyphs := Shape(face, astronaut, Options{})
	if len(glyphs) != 1 {
		t.Fatalf("got %d glyphs, want the composed astronaut", len(glyphs))
	}
	if glyphs[0].Invisible {
		t.Fatal("the composed glyph was hidden — the joiner's cluster must not blank it")
	}
	if glyphs[0].XAdvance <= 0 {
		t.Fatalf("the composed glyph has advance %d, want > 0", glyphs[0].XAdvance)
	}
}

// TestIgnorablesHiddenInEveryWritingMode checks the specialised shapers, which
// return early from Shape, apply the rule too.
func TestIgnorablesHiddenInEveryWritingMode(t *testing.T) {
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face := f.NewFace(32)

	for _, opts := range []Options{{Vertical: true}, {Script: "egyp"}} {
		plain, withWJ := 0, 0
		for _, g := range Shape(face, "ab", opts) {
			plain += g.XAdvance + g.YAdvance
		}
		for _, g := range Shape(face, "a⁠b", opts) {
			withWJ += g.XAdvance + g.YAdvance
		}
		if plain != withWJ {
			t.Errorf("opts %+v: a word joiner added %d units", opts, withWJ-plain)
		}
	}
}
