// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import "unicode"

// isDefaultIgnorable reports whether r is a Default_Ignorable_Code_Point: a
// character that carries instructions for the text process, never a mark on the
// page. The zero-width joiner and non-joiner, the variation selectors, the
// bidi overrides, the word joiner, the soft hyphen, the byte-order mark and the
// language tags are all in this class.
//
// The Unicode derived property is (roughly) Other_Default_Ignorable_Code_Point
// plus Cf plus Variation_Selector, MINUS the prepended concatenation marks.
// Those exceptions matter: U+0600..U+0605, U+06DD, U+070F, U+08E2, U+110BD and
// U+110CD are Cf, yet Arabic and Kaithi genuinely render them (they extend over
// the digits that follow), so hiding them would erase visible text.
func isDefaultIgnorable(r rune) bool {
	switch {
	case r >= 0x0600 && r <= 0x0605, r == 0x06DD, r == 0x070F, r == 0x08E2,
		r == 0x110BD, r == 0x110CD:
		return false
	}
	return unicode.Is(unicode.Cf, r) ||
		unicode.Is(unicode.Variation_Selector, r) ||
		unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r)
}

// hideIgnorables blanks every output glyph that is still standing in for a
// default-ignorable input rune: it takes no width, no offset and is marked
// Invisible so a back-end skips it.
//
// A glyph reaches this point only if shaping did NOT consume it. When a font
// implements an emoji ZWJ sequence, the ligature swallows the joiner and no
// glyph carries its cluster, so nothing is hidden. When the font has no such
// ligature — or no glyph for the character at all — the leftover would
// otherwise be measured and, for a .notdef, measured but not drawn, so the
// reported width and the painted run disagree. U+2060 WORD JOINER did exactly
// that: a .notdef's full advance of empty space in the middle of a word.
//
// This mirrors what HarfBuzz does at the end of its shaping pipeline.
func hideIgnorables(out []Glyph, runes []rune) {
	for i := range out {
		c := out[i].Cluster
		if c < 0 || c >= len(runes) || !isDefaultIgnorable(runes[c]) {
			continue
		}
		out[i].XAdvance = 0
		out[i].YAdvance = 0
		out[i].XOffset = 0
		out[i].YOffset = 0
		out[i].Invisible = true
	}
}
