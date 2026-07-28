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
// The pipeline, per syllable, reproduces HarfBuzz's two reordering passes:
//
//  1. Split two-part dependent vowels (the Bengali/Tamil/Malayalam/... O and AU
//     signs) into their canonical components, so each part reorders to its own
//     position (decomposeIndic).
//  2. Categorize each rune (indicRange table, from cmd/genindic) into a category
//     and a raw positional category, find the reph (the script's encoding: the
//     implicit Ra + halant, Telugu's explicit Ra + halant + ZWJ, or Malayalam's
//     logical-order U+0D4E repha) and the base consonant with the script's
//     base-position model, and — when the
//     cluster is defective (opens with a dependent mark and has no base) — insert
//     a dotted circle to carry the marks (analyzeIndic).
//  3. Initial reordering: a stable sort by ladder position moves pre-base matras
//     ahead of the base and parks the reph Ra at the front of the syllable.
//  4. The basic GSUB features run in order — locl, nukt, akhn, rphf (masked to
//     the reph Ra), rkrf, pref (masked to the pre-base-reordering Ra), blwf,
//     half, pstf, vatu, cjct — and whether rphf and pref actually fired is
//     observed by re-shaping the masked run.
//  5. Final reordering: if rphf fired the reph glyph moves to its per-script slot
//     (otherwise the Ra stays a pre-base consonant); if pref fired the
//     pre-base-reordering Ra moves ahead of the base.
//  6. The presentation GSUB features run — init (masked to the first glyph),
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

// Base-position models: how the base consonant is located within a syllable.
// basePosLast (every configured script but Sinhala) takes the last consonant;
// basePosLastSinhala takes the last consonant too but breaks before a
// ZWJ-joined consonant, the Sinhala rule.
const (
	basePosLast uint8 = iota
	basePosLastSinhala
)

// rephPos is the reorder-ladder slot a reph glyph moves to in the final
// reordering pass. HarfBuzz assigns one per script — Devanagari/Gujarati
// BeforePost, Bengali AfterSub, Gurmukhi BeforeSub, Oriya/Malayalam/Sinhala
// AfterMain, Tamil/Telugu/Kannada AfterPost. Each value is exactly the ladder
// position of its slot, so a rephPos is used directly as a reorder key.
type rephPos uint8

const (
	rephAfterMain  = rephPos(posAfterMain)
	rephBeforeSub  = rephPos(posBeforeSub)
	rephAfterSub   = rephPos(posAfterSub)
	rephBeforePost = rephPos(posBeforePost)
	rephAfterPost  = rephPos(posAfterPost)
)

// rephMode is how a script encodes the reph the shaper reorders. Most scripts
// use the implicit Ra + halant form; Telugu uses an explicit Ra + halant + ZWJ
// sequence; Malayalam encodes a dedicated repha code point (U+0D4E) in logical
// order and needs no ligature to reorder it.
type rephMode uint8

const (
	rephImplicit rephMode = iota // a leading Ra + halant (the common reph)
	rephExplicit                 // a leading Ra + halant + ZWJ (Telugu)
	rephLogRepha                 // a dedicated encoded repha code point (Malayalam)
)

// indicConfig is the per-script Indic configuration.
type indicConfig struct {
	tag      string   // canonical OpenType script tag
	lo, hi   rune     // Unicode block range for auto-detection
	ra       rune     // this script's RA (forms the reph with a following halant)
	rephPos  rephPos  // ladder slot a reph glyph reorders to
	rephMode rephMode // how this script encodes its reph
	basePos  uint8    // base-position model (basePosLast / basePosLastSinhala)
	hasPref  bool     // uses a pre-base-reordering Ra (pref feature) after the base
}

// indicConfigs holds one config per supported Indic script, keyed by canonical
// tag. The blocks are disjoint, so iteration order does not matter. Sinhala uses
// the Sinhala base-position model; Oriya, Tamil, Telugu, Kannada and Malayalam
// carry a pre-base-reordering Ra reordered by the pref feature. Telugu encodes
// its reph explicitly (Ra + halant + ZWJ) and Malayalam as a logical-order repha
// code point (U+0D4E); every other script uses the implicit Ra + halant reph.
var indicConfigs = map[string]indicConfig{
	"deva": {"deva", 0x0900, 0x097F, 0x0930, rephBeforePost, rephImplicit, basePosLast, false},
	"beng": {"beng", 0x0980, 0x09FF, 0x09B0, rephAfterSub, rephImplicit, basePosLast, false},
	"guru": {"guru", 0x0A00, 0x0A7F, 0x0A30, rephBeforeSub, rephImplicit, basePosLast, false},
	"gujr": {"gujr", 0x0A80, 0x0AFF, 0x0AB0, rephBeforePost, rephImplicit, basePosLast, false},
	"orya": {"orya", 0x0B00, 0x0B7F, 0x0B30, rephAfterMain, rephImplicit, basePosLast, true},
	"taml": {"taml", 0x0B80, 0x0BFF, 0x0BB0, rephAfterPost, rephImplicit, basePosLast, true},
	"telu": {"telu", 0x0C00, 0x0C7F, 0x0C30, rephAfterPost, rephExplicit, basePosLast, true},
	"knda": {"knda", 0x0C80, 0x0CFF, 0x0CB0, rephAfterPost, rephImplicit, basePosLast, true},
	"mlym": {"mlym", 0x0D00, 0x0D7F, 0x0D30, rephAfterMain, rephLogRepha, basePosLast, true},
	"sinh": {"sinh", 0x0D80, 0x0DFF, 0x0DBB, rephAfterMain, rephImplicit, basePosLastSinhala, false},
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
	"sinh": "sinh", "snh2": "sinh",
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

// isJoinerCat reports whether a category is a joiner (ZWJ or ZWNJ). A joiner
// holds the following starter in the same syllable, and — after a reph's halant
// — a ZWJ distinguishes the explicit-repha form from the implicit one.
func isJoinerCat(cat uint8) bool { return cat == catZWJ || cat == catZWNJ }

// hasConsonantFrom reports whether cats[from:end] holds a base-eligible
// consonant — the consonant a reph needs after it to have a base.
func hasConsonantFrom(cats []uint8, from, end int) bool {
	for i := from; i < end; i++ {
		if isIndicConsonant(cats[i]) {
			return true
		}
	}
	return false
}

// isIndicBase reports whether a category can serve as a syllable base when no
// consonant does: an independent vowel, a placeholder or a dotted circle.
func isIndicBase(cat uint8) bool {
	return cat == catV || cat == catPlaceholder || cat == catDottedCircle
}

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

// indicDecomp holds the canonical (Unicode NFD) decompositions of the Indic
// two-part dependent vowels — the "split matras" whose glyphs render with a
// pre-base part and an above/below/post part. HarfBuzz decomposes these during
// normalization so each part reaches its own reorder position; the values are
// fully decomposed (no value contains a further-decomposable code point).
var indicDecomp = map[rune][]rune{
	// Bengali
	0x09CB: {0x09C7, 0x09BE}, 0x09CC: {0x09C7, 0x09D7},
	// Oriya
	0x0B48: {0x0B47, 0x0B56}, 0x0B4B: {0x0B47, 0x0B3E}, 0x0B4C: {0x0B47, 0x0B57},
	// Tamil
	0x0BCA: {0x0BC6, 0x0BBE}, 0x0BCB: {0x0BC7, 0x0BBE}, 0x0BCC: {0x0BC6, 0x0BD7},
	// Telugu
	0x0C48: {0x0C46, 0x0C56},
	// Kannada
	0x0CC0: {0x0CBF, 0x0CD5}, 0x0CC7: {0x0CC6, 0x0CD5}, 0x0CC8: {0x0CC6, 0x0CD6},
	0x0CCA: {0x0CC6, 0x0CC2}, 0x0CCB: {0x0CC6, 0x0CC2, 0x0CD5},
	// Malayalam
	0x0D4A: {0x0D46, 0x0D3E}, 0x0D4B: {0x0D47, 0x0D3E}, 0x0D4C: {0x0D46, 0x0D57},
}

// decomposeIndic expands every split matra in runes into its canonical
// components, returning the expanded rune sequence and, for each expanded rune,
// the index of the source rune it derives from (its cluster). A rune that does
// not decompose is carried through unchanged with its own index.
func decomposeIndic(runes []rune) (out []rune, clusters []int) {
	for i, r := range runes {
		if parts, ok := indicDecomp[r]; ok {
			for _, p := range parts {
				out = append(out, p)
				clusters = append(clusters, i)
			}
			continue
		}
		out = append(out, r)
		clusters = append(clusters, i)
	}
	return out, clusters
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
		case isIndicStarter(cats[i]) && cats[i-1] != catH &&
			cats[i-1] != catRepha && !isJoinerCat(cats[i-1]):
			// A starter breaks a syllable unless the previous rune holds it: a
			// halant, a leading repha (Malayalam) or a joiner (the ZWJ of an
			// explicit reph, or a conjunct-forming joiner).
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

// findIndicBase locates the base consonant in cats[from:end] under the given
// base-position model, returning its index or -1 when there is no consonant.
func findIndicBase(cats []uint8, from, end int, mode uint8) int {
	if mode == basePosLastSinhala {
		base := -1
		for i := from; i < end; i++ {
			if isIndicConsonant(cats[i]) {
				if i > from && cats[i-1] == catZWJ {
					break
				}
				base = i
			}
		}
		return base
	}
	for i := end - 1; i >= from; i-- {
		if isIndicConsonant(cats[i]) {
			return i
		}
	}
	return -1
}

// indicSyllable is the analyzed form of one syllable: per-glyph categories, raw
// positions and initial ladder positions, plus the base index, the reph flag
// and the pre-base-reordering Ra index (prefIdx, -1 when none).
type indicSyllable struct {
	cats    []uint8
	poss    []uint8
	base    int
	reph    bool
	prefIdx int
}

// analyzeIndic categorizes one syllable's runes, finds its reph and base, and
// assigns each glyph its initial ladder position. When the syllable is defective
// (indic, but with no consonant, vowel or placeholder to be the base and opening
// with a dependent mark) it prepends a dotted circle to carry the marks; the
// returned runes and clusters reflect that insertion. It reports indic=false for
// a span with no Indic character at all.
func analyzeIndic(runes []rune, clusters []int, cfg indicConfig) (lr []rune, lc []int, syl indicSyllable, indic bool) {
	lr = append([]rune(nil), runes...)
	lc = append([]int(nil), clusters...)
	if !anyIndic(lr) {
		poss := make([]uint8, len(lr))
		for i := range poss {
			poss[i] = posEnd
		}
		return lr, lc, indicSyllable{poss: poss, base: -1, prefIdx: -1}, false
	}

	syl = classifyIndic(lr, cfg)
	if syl.base < 0 {
		// Defective cluster: insert a dotted circle at the front as the base and
		// re-classify around it.
		lr = append([]rune{0x25CC}, lr...)
		lc = append([]int{firstCluster(clusters)}, lc...)
		syl = classifyIndic(lr, cfg)
	}
	return lr, lc, syl, true
}

// anyIndic reports whether any rune categorizes to something other than catX.
func anyIndic(runes []rune) bool {
	for _, r := range runes {
		if c, _ := indicCat(r); c != catX {
			return true
		}
	}
	return false
}

// firstCluster returns the first cluster index, or 0 for an empty slice.
func firstCluster(clusters []int) int {
	if len(clusters) == 0 {
		return 0
	}
	return clusters[0]
}

// classifyIndic assigns categories, raw positions, ladder positions, the base,
// the reph flag and the pre-base-reordering Ra of a syllable's runes.
func classifyIndic(runes []rune, cfg indicConfig) indicSyllable {
	n := len(runes)
	cats := make([]uint8, n)
	raws := make([]uint8, n)
	for i, r := range runes {
		cats[i], raws[i] = indicCat(r)
	}

	// Reph detection, per the script's encoding mode: the implicit Ra + halant,
	// Telugu's explicit Ra + halant + ZWJ, or Malayalam's dedicated logical-order
	// repha code point. Every form needs a consonant after it to be the base, and
	// from marks the first rune past the reph (the parked prefix).
	reph := false
	from := 0
	switch cfg.rephMode {
	case rephImplicit:
		if n >= 3 && runes[0] == cfg.ra && cats[1] == catH &&
			!isJoinerCat(cats[2]) && hasConsonantFrom(cats, 2, n) {
			reph, from = true, 2
		}
	case rephExplicit:
		if n >= 4 && runes[0] == cfg.ra && cats[1] == catH &&
			cats[2] == catZWJ && hasConsonantFrom(cats, 3, n) {
			reph, from = true, 3
		}
	case rephLogRepha:
		if n >= 2 && cats[0] == catRepha && hasConsonantFrom(cats, 1, n) {
			reph, from = true, 1
		}
	}

	base := findIndicBase(cats, from, n, cfg.basePos)

	// Pre-base-reordering Ra: for the scripts that carry one, a trailing halant +
	// Ra is excluded from the base and reordered by the pref feature instead.
	prefIdx := -1
	if cfg.hasPref && base > from && runes[base] == cfg.ra && cats[base-1] == catH {
		prefIdx = base
		base = findIndicBase(cats, from, base-1, cfg.basePos)
	}

	if base < 0 {
		// No consonant: the base is the first independent vowel, placeholder or
		// dotted circle, else there is no base (a defective cluster).
		for i := 0; i < n; i++ {
			if isIndicBase(cats[i]) {
				base = i
				break
			}
		}
	}

	poss := make([]uint8, n)
	for i := 0; i < n; i++ {
		switch {
		case reph && i < from:
			poss[i] = posRaToReph
		case i < base:
			poss[i] = posPreC
		case i == base:
			poss[i] = posBaseC
		default:
			poss[i] = afterBasePos(cats[i], raws[i])
		}
	}
	return indicSyllable{cats: cats, poss: poss, base: base, reph: reph, prefIdx: prefIdx}
}

// reorderIndic stably permutes gl and its parallel track slice by key(track[i]).
func reorderIndic(gl []opentype.GlyphIndex, track []int, key func(int) uint8) {
	idx := make([]int, len(gl))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return key(track[idx[a]]) < key(track[idx[b]]) })
	ng := make([]opentype.GlyphIndex, len(gl))
	nt := make([]int, len(track))
	for k, j := range idx {
		ng[k] = gl[j]
		nt[k] = track[j]
	}
	copy(gl, ng)
	copy(track, nt)
}

// maskForTrack returns a position mask selecting the glyphs whose track id is
// id, so a masked feature applies exactly at that logical glyph wherever the
// reordering has moved it.
func maskForTrack(track []int, id int) []bool {
	m := make([]bool, len(track))
	for i, t := range track {
		if t == id {
			m[i] = true
		}
	}
	return m
}

// applyIndicFeature applies a single masked GSUB feature to the run and reports
// whether it changed the glyphs (fired). track is threaded through so the caller
// keeps its logical-index mapping.
func applyIndicFeature(g *opentype.GSUB, gl []opentype.GlyphIndex, track []int, tag string, mask []bool) ([]opentype.GlyphIndex, []int, bool) {
	ng, nt := g.ApplyMaskedTracked(gl, track, []opentype.FeatureApp{{Tag: tag, Positions: mask}})
	return ng, nt, glyphsChanged(gl, ng)
}

// glyphsChanged reports whether two glyph runs differ in length or content.
func glyphsChanged(a, b []opentype.GlyphIndex) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

// shapeIndic runs the full Indic GSUB pipeline over the run, one syllable at a
// time, and returns the substituted, reordered run with its clusters. It rebuilds
// the glyph run from the (split-matra-decomposed) runes via the font's cmap, so
// the incoming run/clusters from Shape are not needed here.
func shapeIndic(font *opentype.Font, g *opentype.GSUB, runes []rune, cfg indicConfig, user []string) ([]opentype.GlyphIndex, []int) {
	dRunes, dClusters := decomposeIndic(runes)
	var outG []opentype.GlyphIndex
	var outC []int
	for _, sp := range segmentIndic(dRunes) {
		gl, cl := shapeIndicSyllable(font, g, dRunes[sp[0]:sp[1]], dClusters[sp[0]:sp[1]], cfg, user)
		outG = append(outG, gl...)
		outC = append(outC, cl...)
	}
	return outG, outC
}

// shapeIndicSyllable shapes one syllable span given its decomposed runes and
// their clusters (source rune indices). It maps the runes to glyphs, runs the
// two-pass reordering and the GSUB feature pipeline, and returns the shaped
// glyphs with each glyph mapped back to its source cluster.
func shapeIndicSyllable(font *opentype.Font, g *opentype.GSUB, runes []rune, clusters []int, cfg indicConfig, user []string) ([]opentype.GlyphIndex, []int) {
	lr, lc, syl, indic := analyzeIndic(runes, clusters, cfg)

	// Map runes to glyphs and set up the logical-index tracking id per glyph.
	gl := make([]opentype.GlyphIndex, len(lr))
	track := make([]int, len(lr))
	for i, r := range lr {
		gl[i], _ = font.GlyphIndex(r)
		track[i] = i
	}
	if !indic {
		return gl, lc // non-Indic run: pass through untouched.
	}

	lpos := syl.poss

	// Initial reordering: pre-base matras before the base, the reph Ra at front.
	reorderIndic(gl, track, func(id int) uint8 { return lpos[id] })

	// Basic features. rphf is masked to the reph Ra; pref to the
	// pre-base-reordering Ra; the rest run over the whole syllable. Whether rphf
	// and pref fired drives the final reordering.
	gl, track = g.ApplyMaskedTracked(gl, track, []opentype.FeatureApp{{Tag: "locl"}, {Tag: "nukt"}, {Tag: "akhn"}})
	rphfFired := false
	if syl.reph {
		gl, track, rphfFired = applyIndicFeature(g, gl, track, "rphf", maskForTrack(track, 0))
	}
	gl, track = g.ApplyMaskedTracked(gl, track, []opentype.FeatureApp{{Tag: "rkrf"}})
	prefFired := false
	if syl.prefIdx >= 0 {
		gl, track, prefFired = applyIndicFeature(g, gl, track, "pref", maskForTrack(track, syl.prefIdx))
	}
	gl, track = g.ApplyMaskedTracked(gl, track, []opentype.FeatureApp{
		{Tag: "blwf"}, {Tag: "half"}, {Tag: "pstf"}, {Tag: "vatu"}, {Tag: "cjct"},
	})

	// Final reordering: the reph moves to its per-script slot — when rphf fired
	// (the implicit and explicit modes ligate the Ra + halant), or, for a
	// logical-order repha, simply because it was found (the code point is already
	// the reph glyph, so no ligature is needed to reorder it). A fired pre-base-
	// reordering Ra moves ahead of the base; everything else keeps its initial
	// ladder position.
	rephMoves := syl.reph && (rphfFired || cfg.rephMode == rephLogRepha)
	reorderIndic(gl, track, func(id int) uint8 {
		switch {
		case rephMoves && id == 0:
			return uint8(cfg.rephPos)
		case prefFired && id == syl.prefIdx:
			return posPreC
		default:
			return lpos[id]
		}
	})

	// Presentation features. init is masked to the first glyph of the reordered
	// syllable (a substitution or ligature never empties a non-empty run, so
	// there is always a first glyph here).
	initMask := make([]bool, len(gl))
	initMask[0] = true
	pres := []opentype.FeatureApp{
		{Tag: "init", Positions: initMask}, {Tag: "pres"}, {Tag: "abvs"},
		{Tag: "blws"}, {Tag: "psts"}, {Tag: "haln"},
	}
	gl, track = g.ApplyMaskedTracked(gl, track, appendUser(pres, user))

	// Map each output glyph back to its source cluster.
	oc := make([]int, len(gl))
	for i, t := range track {
		oc[i] = lc[t]
	}
	return gl, oc
}
