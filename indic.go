// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"sort"

	"github.com/go-opentype/opentype"
)

// Indic shaping (the HarfBuzz "indic" shaper, dev2/bng2/... feature model).
//
// The pipeline, per syllable:
//
//  1. Categorize each rune (indicRange table, from cmd/genindic) into a category
//     and a raw positional category.
//  2. Find the reph (a leading Ra + halant) and the base consonant (the last
//     consonant, BASE_POS_LAST), and assign every glyph a reorder position on a
//     single ladder from POS_PRE_M (pre-base matra) through POS_BASE_C to
//     POS_SMVD (syllable modifiers).
//  3. Apply the basic GSUB features in order — locl, nukt, akhn, rphf (masked to
//     the reph only), rkrf, pref, blwf, half, pstf, vatu, cjct.
//  4. Reorder: a stable sort by ladder position moves pre-base matras before the
//     base and the reph glyph to its script-specific position.
//  5. Apply the presentation GSUB features — init (masked to the first glyph),
//     pres, abvs, blws, psts, haln.
//
// GPOS (kern, dist, abvm, blwm, mark, mkmk) is then run over the whole run by
// Shape, as for the other scripts.

// indicRange maps an inclusive code point range to an Indic category and a raw
// positional category. The generated indicData table is a sorted list of these.
type indicRange struct {
	lo, hi   rune
	cat, pos uint8
}

// Indic categories (the shaper's internal Indic_Syllabic_Category folding).
const (
	catX            uint8 = iota // other / non-Indic
	catC                         // consonant
	catV                         // independent vowel
	catN                         // nukta
	catH                         // halant / virama / stacker
	catZWNJ                      // zero-width non-joiner
	catZWJ                       // zero-width joiner
	catM                         // dependent vowel (matra)
	catSM                        // syllable modifier (bindu, visarga, ...)
	catA                         // cantillation / tone mark
	catPlaceholder               // number, placeholder, head letter
	catDottedCircle              // U+25CC
	catRS                        // register shifter
	catRepha                     // preceding/succeeding repha consonant
	catCM                        // consonant medial / final / subjoined
	catSymbol                    // avagraha and the like
	catCS                        // consonant with stacker
)

// Reorder positions, low to high; a stable sort by this ladder is the whole of
// initial reordering. posRaToReph and the several "before/after" slots are used
// as per-script reph targets in indicConfigs.
const (
	posStart uint8 = iota
	posRaToReph
	posPreM
	posPreC
	posBaseC
	posAfterMain
	posAboveC
	posBeforeSub
	posBelowC
	posAfterSub
	posBeforePost
	posPostC
	posAfterPost
	posSMVD
	posEnd
)

// indicConfig is the per-script Indic configuration.
type indicConfig struct {
	tag     string // canonical OpenType script tag
	lo, hi  rune   // Unicode block range for auto-detection
	ra      rune   // this script's RA (forms the reph with a following halant)
	rephPos uint8  // ladder position a reph glyph reorders to
}

// indicConfigs holds one config per supported Indic script, keyed by canonical
// tag. The blocks are disjoint, so iteration order does not matter.
var indicConfigs = map[string]indicConfig{
	"deva": {"deva", 0x0900, 0x097F, 0x0930, posBeforePost},
	"beng": {"beng", 0x0980, 0x09FF, 0x09B0, posAfterSub},
	"guru": {"guru", 0x0A00, 0x0A7F, 0x0A30, posBeforeSub},
	"gujr": {"gujr", 0x0A80, 0x0AFF, 0x0AB0, posBeforePost},
	"orya": {"orya", 0x0B00, 0x0B7F, 0x0B30, posAfterMain},
	"taml": {"taml", 0x0B80, 0x0BFF, 0x0BB0, posAfterPost},
	"telu": {"telu", 0x0C00, 0x0C7F, 0x0C30, posAfterPost},
	"knda": {"knda", 0x0C80, 0x0CFF, 0x0CB0, posAfterPost},
	"mlym": {"mlym", 0x0D00, 0x0D7F, 0x0D30, posAfterMain},
}

// indicAlias maps every accepted script tag — the old (deva) and the v2 (dev2)
// forms — to a canonical tag in indicConfigs.
var indicAlias = map[string]string{
	"deva": "deva", "dev2": "deva",
	"beng": "beng", "bng2": "beng",
	"guru": "guru", "gur2": "guru",
	"gujr": "gujr", "gjr2": "gujr",
	"orya": "orya", "ory2": "orya",
	"taml": "taml", "tml2": "taml",
	"telu": "telu", "tel2": "telu",
	"knda": "knda", "knd2": "knda",
	"mlym": "mlym", "mlm2": "mlym",
}

// indicConfigForTag resolves a script tag (old or v2 form, case-insensitive) to
// its config.
func indicConfigForTag(tag string) (indicConfig, bool) {
	canon, ok := indicAlias[lower(tag)]
	if !ok {
		return indicConfig{}, false
	}
	return indicConfigs[canon], true
}

// indicConfigForRune returns the config whose Unicode block contains r.
func indicConfigForRune(r rune) (indicConfig, bool) {
	for _, c := range indicConfigs {
		if r >= c.lo && r <= c.hi {
			return c, true
		}
	}
	return indicConfig{}, false
}

// indicConfigForRunes returns the config for the first Indic rune in runes.
func indicConfigForRunes(runes []rune) (indicConfig, bool) {
	for _, r := range runes {
		if c, ok := indicConfigForRune(r); ok {
			return c, true
		}
	}
	return indicConfig{}, false
}

// lower lowercases an ASCII tag without importing strings for the hot path.
func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// lookupIndic returns the Indic category and raw position of r, defaulting to
// (catX, posEnd) for code points outside every range.
func lookupIndic(r rune) (cat, pos uint8) {
	lo, hi := 0, len(indicData)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		e := indicData[mid]
		switch {
		case r < e.lo:
			hi = mid
		case r > e.hi:
			lo = mid + 1
		default:
			return e.cat, e.pos
		}
	}
	return catX, posEnd
}

// indicCat returns the effective category of r, applying the code-point
// overrides the table does not carry (the joiners and the dotted circle).
func indicCat(r rune) (cat, pos uint8) {
	cat, pos = lookupIndic(r)
	switch r {
	case 0x200C:
		cat = catZWNJ
	case 0x200D:
		cat = catZWJ
	case 0x25CC:
		cat = catDottedCircle
	}
	return cat, pos
}

// isIndicConsonant reports whether a category can act as the base consonant.
func isIndicConsonant(cat uint8) bool { return cat == catC || cat == catCS }

// isIndicStarter reports whether a category begins a new syllable when it is not
// bound to the previous one by a halant.
func isIndicStarter(cat uint8) bool {
	switch cat {
	case catC, catCS, catV, catRepha, catPlaceholder, catDottedCircle:
		return true
	default:
		return false
	}
}

// segmentIndic splits runes into syllable spans [start,end). A span breaks
// before a starter that is not held to the previous rune by a halant, and
// before/after a run of non-Indic (catX) characters.
func segmentIndic(runes []rune) [][2]int {
	if len(runes) == 0 {
		return nil
	}
	cats := make([]uint8, len(runes))
	for i, r := range runes {
		cats[i], _ = indicCat(r)
	}
	var spans [][2]int
	start := 0
	for i := 1; i < len(runes); i++ {
		brk := false
		switch {
		case isIndicStarter(cats[i]) && cats[i-1] != catH:
			brk = true
		case cats[i] == catX && cats[i-1] != catX:
			brk = true
		}
		if brk {
			spans = append(spans, [2]int{start, i})
			start = i
		}
	}
	return append(spans, [2]int{start, len(runes)})
}

// afterBasePos gives the ladder position of a glyph that sits after the base:
// matras by their raw position, syllable modifiers and cantillation marks at
// the very end, halant/nukta and post-base consonants below the base.
func afterBasePos(cat, raw uint8) uint8 {
	switch cat {
	case catM:
		if raw == posEnd {
			return posPostC
		}
		return raw
	case catSM, catA:
		return posSMVD
	case catH:
		return posBelowC
	case catN:
		return posAboveC
	case catC, catCS, catCM:
		return posBelowC
	default:
		return posEnd
	}
}

// analyzeSyllable categorizes runes[start:end], finds the reph and base, and
// returns each local glyph's ladder position plus whether the span has a reph
// and whether it contains any Indic character at all.
func analyzeSyllable(runes []rune, start, end int, cfg indicConfig) (pos []uint8, reph, indic bool) {
	n := end - start
	cats := make([]uint8, n)
	raws := make([]uint8, n)
	for i := 0; i < n; i++ {
		cats[i], raws[i] = indicCat(runes[start+i])
		if cats[i] != catX {
			indic = true
		}
	}
	pos = make([]uint8, n)
	if !indic {
		for i := range pos {
			pos[i] = posEnd
		}
		return pos, false, false
	}

	// Reph: a leading Ra + halant with a consonant to be the base after them.
	from := 0
	if n >= 3 && runes[start] == cfg.ra && cats[1] == catH {
		for i := 2; i < n; i++ {
			if isIndicConsonant(cats[i]) {
				reph = true
				from = 2
				break
			}
		}
	}

	// Base: the last consonant at or after the reph (BASE_POS_LAST).
	base := -1
	for i := n - 1; i >= from; i-- {
		if isIndicConsonant(cats[i]) {
			base = i
			break
		}
	}
	if base < 0 {
		// No consonant to be the base: a vowel (or standalone) syllable. The
		// base is the first independent vowel or placeholder, else the start.
		base = 0
		for i := 0; i < n; i++ {
			if cats[i] == catV || cats[i] == catPlaceholder {
				base = i
				break
			}
		}
	}

	for i := 0; i < n; i++ {
		switch {
		case reph && i < 2:
			pos[i] = cfg.rephPos
		case i < base:
			pos[i] = posPreC
		case i == base:
			pos[i] = posBaseC
		default:
			pos[i] = afterBasePos(cats[i], raws[i])
		}
	}
	return pos, reph, true
}

// reorderByPos stably reorders a glyph run and its clusters by the ladder
// position of each glyph's source cluster, the Indic initial-reordering move.
func reorderByPos(gl []opentype.GlyphIndex, cl []int, posOf map[int]uint8) ([]opentype.GlyphIndex, []int) {
	idx := make([]int, len(gl))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return posOf[cl[idx[a]]] < posOf[cl[idx[b]]] })
	og := make([]opentype.GlyphIndex, len(gl))
	oc := make([]int, len(cl))
	for k, j := range idx {
		og[k] = gl[j]
		oc[k] = cl[j]
	}
	return og, oc
}

// shapeIndic runs the full Indic GSUB pipeline over the run, one syllable at a
// time, and returns the substituted, reordered run with its clusters. The input
// clusters are the identity (glyph i comes from rune i), as Shape passes them.
func shapeIndic(g *opentype.GSUB, run []opentype.GlyphIndex, clusters []int, runes []rune, cfg indicConfig, user []string) ([]opentype.GlyphIndex, []int) {
	var outG []opentype.GlyphIndex
	var outC []int
	for _, sp := range segmentIndic(runes) {
		gl := append([]opentype.GlyphIndex(nil), run[sp[0]:sp[1]]...)
		cl := append([]int(nil), clusters[sp[0]:sp[1]]...)
		gl, cl = shapeIndicSyllable(g, gl, cl, runes, sp[0], sp[1], cfg, user)
		outG = append(outG, gl...)
		outC = append(outC, cl...)
	}
	return outG, outC
}

// shapeIndicSyllable shapes one syllable span. gl/cl are the syllable's glyphs
// and (global) clusters; start/end delimit the span in runes.
func shapeIndicSyllable(g *opentype.GSUB, gl []opentype.GlyphIndex, cl []int, runes []rune, start, end int, cfg indicConfig, user []string) ([]opentype.GlyphIndex, []int) {
	pos, reph, indic := analyzeSyllable(runes, start, end, cfg)
	if !indic {
		return gl, cl // non-Indic run: pass through untouched.
	}
	posOf := make(map[int]uint8, len(pos))
	for i, p := range pos {
		posOf[start+i] = p
	}

	// Basic features. rphf is masked to the reph's leading Ra (local index 0)
	// so a medial Ra+halant conjunct is not turned into a reph.
	rphfMask := make([]bool, len(gl))
	if reph {
		rphfMask[0] = true
	}
	basic := []opentype.FeatureApp{
		{Tag: "locl"}, {Tag: "nukt"}, {Tag: "akhn"},
		{Tag: "rphf", Positions: rphfMask},
		{Tag: "rkrf"}, {Tag: "pref"}, {Tag: "blwf"}, {Tag: "half"},
		{Tag: "pstf"}, {Tag: "vatu"}, {Tag: "cjct"},
	}
	gl, cl = g.ApplyMaskedTracked(gl, cl, basic)

	gl, cl = reorderByPos(gl, cl, posOf)

	// Presentation features. init is masked to the first glyph of the reordered
	// syllable (a substitution or ligature never empties a non-empty run, so
	// there is always a first glyph here).
	initMask := make([]bool, len(gl))
	initMask[0] = true
	pres := []opentype.FeatureApp{
		{Tag: "init", Positions: initMask}, {Tag: "pres"}, {Tag: "abvs"},
		{Tag: "blws"}, {Tag: "psts"}, {Tag: "haln"},
	}
	return g.ApplyMaskedTracked(gl, cl, appendUser(pres, user))
}
