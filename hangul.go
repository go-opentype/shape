// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

// Hangul conjoining-jamo composition.
//
// Korean text most commonly arrives as precomposed Hangul syllables
// (U+AC00–U+D7A3), which the default shaping path maps straight through the
// cmap. Text can also arrive as sequences of *conjoining jamo* — a leading
// consonant (L), a vowel (V) and an optional trailing consonant (T) from the
// Hangul Jamo block (U+1100–U+11FF) — which must be composed into their
// precomposed syllable before rendering (a font rarely carries standalone jamo
// glyphs). This is the algorithmic L·V·T → syllable composition of the Unicode
// Standard (the Hangul part of the NFC algorithm); it runs as a rune-level
// pre-pass in Shape, so the composed syllables then flow through the ordinary
// cmap + GSUB/GPOS pipeline.
//
// Full old-Hangul shaping (the ljmo/vjmo/tjmo positional-jamo GSUB features for
// fonts that render decomposed jamo, and Hangul-Jamo-Extended blocks) is not
// reproduced; those remaining jamo pass through unchanged. Documented scope.
const (
	hangulLBase  = 0x1100 // first leading consonant (choseong)
	hangulVBase  = 0x1161 // first vowel (jungseong)
	hangulTBase  = 0x11A7 // T index 0 = "no trailing"; real T starts at 0x11A8
	hangulLCount = 19
	hangulVCount = 21
	hangulTCount = 28
	hangulSBase  = 0xAC00                      // first precomposed syllable
	hangulNCount = hangulVCount * hangulTCount // 588
	hangulSCount = hangulLCount * hangulNCount // 11172 precomposed syllables
)

// hasHangulJamo reports whether runes contains any conjoining Hangul jamo
// (U+1100–U+11FF) — the only trigger for composeHangul to do work.
func hasHangulJamo(runes []rune) bool {
	for _, r := range runes {
		if r >= 0x1100 && r <= 0x11FF {
			return true
		}
	}
	return false
}

// composeHangul folds conjoining jamo (L+V and L+V+T, plus an already-composed
// LV syllable followed by a trailing T) into precomposed Hangul syllables,
// leaving every other rune — and any jamo that does not form a valid pair —
// untouched. With no conjoining jamo present it returns runes unchanged (the
// common precomposed-Korean and non-Korean case).
func composeHangul(runes []rune) []rune {
	if !hasHangulJamo(runes) {
		return runes
	}
	out := make([]rune, 0, len(runes))
	for _, r := range runes {
		if len(out) > 0 {
			last := out[len(out)-1]
			// L + V -> LV syllable.
			if last >= hangulLBase && last < hangulLBase+hangulLCount &&
				r >= hangulVBase && r < hangulVBase+hangulVCount {
				li := int(last - hangulLBase)
				vi := int(r - hangulVBase)
				out[len(out)-1] = rune(hangulSBase + (li*hangulVCount+vi)*hangulTCount)
				continue
			}
			// LV syllable + T -> LVT syllable (last is a T-less syllable).
			if last >= hangulSBase && last < hangulSBase+hangulSCount &&
				int(last-hangulSBase)%hangulTCount == 0 &&
				r > hangulTBase && r < hangulTBase+hangulTCount {
				ti := int(r - hangulTBase)
				out[len(out)-1] = last + rune(ti)
				continue
			}
		}
		out = append(out, r)
	}
	return out
}
