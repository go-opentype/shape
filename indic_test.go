// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"reflect"
	"testing"

	"github.com/go-opentype/fonts/notosansdevanagari"
	"github.com/go-opentype/opentype"
)

func notoDevanagari(t *testing.T) *opentype.Font {
	t.Helper()
	f, err := opentype.Parse(notosansdevanagari.TTF)
	if err != nil {
		t.Fatalf("parse Noto Sans Devanagari: %v", err)
	}
	return f
}

// naiveGIDs is the per-rune cmap mapping with no shaping.
func naiveGIDs(f *opentype.Font, s string) []opentype.GlyphIndex {
	var out []opentype.GlyphIndex
	for _, r := range s {
		g, _ := f.GlyphIndex(r)
		out = append(out, g)
	}
	return out
}

func gidsOf(gs []Glyph) []opentype.GlyphIndex {
	out := make([]opentype.GlyphIndex, len(gs))
	for i, g := range gs {
		out[i] = g.GID
	}
	return out
}

// visualIndexOfCluster returns the position of the (first) glyph deriving from
// source rune c in the shaped run, or -1.
func visualIndexOfCluster(gs []Glyph, c int) int {
	for i, g := range gs {
		if g.Cluster == c {
			return i
		}
	}
	return -1
}

// TestRealIndicPreBaseMatra shapes "कि" (ka + vowel sign I). The I matra is a
// left/pre-base matra: it must reorder before the base consonant, and the run
// must differ from the naive per-rune mapping in order (and, via the pres
// feature, in glyph ids).
func TestRealIndicPreBaseMatra(t *testing.T) {
	f := notoDevanagari(t)
	face := f.NewFace(64)
	const word = "कि" // U+0915 U+093F
	got := Shape(face, word, Options{})
	if len(got) < 2 {
		t.Fatalf("shaped %d glyphs, want >= 2: %+v", len(got), got)
	}
	// The matra (source rune 1) must come before the base ka (source rune 0).
	mi := visualIndexOfCluster(got, 1)
	ki := visualIndexOfCluster(got, 0)
	if mi < 0 || ki < 0 {
		t.Fatalf("clusters not both present: %+v", got)
	}
	if mi >= ki {
		t.Errorf("pre-base matra not reordered before base: matra@%d base@%d %+v", mi, ki, got)
	}
	naive := naiveGIDs(f, word)
	if reflect.DeepEqual(gidsOf(got), naive) {
		t.Errorf("shaped run equals naive mapping %v; no shaping happened", naive)
	}
	t.Logf("कि naive=%v shaped=%v", naive, gidsOf(got))
}

// TestRealIndicReph shapes "र्क" (Ra + virama + ka): the leading Ra+virama forms
// a reph that must ligate (rphf) and reorder to after the base consonant.
func TestRealIndicReph(t *testing.T) {
	f := notoDevanagari(t)
	face := f.NewFace(64)
	const word = "र्क" // U+0930 U+094D U+0915
	got := Shape(face, word, Options{})
	if len(got) < 2 {
		t.Fatalf("shaped %d glyphs, want >= 2: %+v", len(got), got)
	}
	reph := visualIndexOfCluster(got, 0) // reph derives from the Ra (rune 0)
	base := visualIndexOfCluster(got, 2) // base ka (rune 2)
	if reph < 0 || base < 0 {
		t.Fatalf("reph/base clusters not present: %+v", got)
	}
	if reph <= base {
		t.Errorf("reph not reordered after base: reph@%d base@%d %+v", reph, base, got)
	}
	naive := naiveGIDs(f, word)
	if reflect.DeepEqual(gidsOf(got), naive) || len(got) == len(naive) {
		t.Errorf("reph did not ligate: shaped=%v naive=%v", gidsOf(got), naive)
	}
	t.Logf("र्क naive=%v shaped=%v", naive, gidsOf(got))
}

// TestRealIndicConjunct shapes "क्क" (ka + virama + ka): the half feature must
// collapse the pre-base ka+virama into a half form, shortening the run.
func TestRealIndicConjunct(t *testing.T) {
	f := notoDevanagari(t)
	got := Shape(f.NewFace(64), "क्क", Options{Script: "dev2"})
	if len(got) >= 3 {
		t.Errorf("conjunct not formed, run not shortened: %+v", got)
	}
	if len(got) == 0 {
		t.Fatalf("no glyphs")
	}
}

// TestRealIndicBinduAndPassthrough shapes ka + anusvara (a syllable modifier,
// stays after the base) mixed with a non-Indic space + Latin, exercising the
// multi-syllable segmentation and the non-Indic passthrough.
func TestRealIndicBinduAndPassthrough(t *testing.T) {
	f := notoDevanagari(t)
	got := Shape(f.NewFace(48), "कं a", Options{})
	if len(got) < 3 {
		t.Fatalf("shaped %d glyphs, want >= 3: %+v", len(got), got)
	}
	// The bindu (cluster 1) stays after the base ka (cluster 0).
	if b, k := visualIndexOfCluster(got, 1), visualIndexOfCluster(got, 0); b >= 0 && k >= 0 && b < k {
		t.Errorf("bindu reordered before base unexpectedly: %+v", got)
	}
	last := got[len(got)-1]
	if last.Cluster != 3 { // the Latin 'a'
		t.Errorf("last glyph cluster = %d, want 3 (the Latin a): %+v", last.Cluster, got)
	}
}

// TestRealIndicGPOS confirms the Indic GPOS feature set positions the run (a
// font with GPOS yields non-nil positions; advances are positive).
func TestRealIndicGPOS(t *testing.T) {
	f := notoDevanagari(t)
	got := Shape(f.NewFace(64), "क", Options{})
	if len(got) != 1 || got[0].XAdvance <= 0 {
		t.Errorf("single consonant advance not positive: %+v", got)
	}
}

func TestIndicConfigForTag(t *testing.T) {
	for _, tag := range []string{"deva", "dev2", "BENG", "bng2", "mlm2"} {
		if _, ok := indicConfigForTag(tag); !ok {
			t.Errorf("indicConfigForTag(%q) not found", tag)
		}
	}
	if _, ok := indicConfigForTag("latn"); ok {
		t.Errorf("indicConfigForTag(latn) unexpectedly found")
	}
}

func TestIndicConfigForRune(t *testing.T) {
	if c, ok := indicConfigForRune(0x0915); !ok || c.tag != "deva" {
		t.Errorf("0x0915 -> %v,%v want deva", c.tag, ok)
	}
	if c, ok := indicConfigForRune(0x0D30); !ok || c.tag != "mlym" {
		t.Errorf("0x0D30 -> %v,%v want mlym", c.tag, ok)
	}
	if _, ok := indicConfigForRune('a'); ok {
		t.Errorf("'a' should not resolve to an Indic config")
	}
}

func TestIndicConfigForRunes(t *testing.T) {
	if c, ok := indicConfigForRunes([]rune("abक")); !ok || c.tag != "deva" {
		t.Errorf("mixed runes -> %v,%v want deva", c.tag, ok)
	}
	if _, ok := indicConfigForRunes([]rune("abc")); ok {
		t.Errorf("latin runes should not resolve")
	}
}

func TestLower(t *testing.T) {
	if got := lower("Dev2"); got != "dev2" {
		t.Errorf("lower(Dev2) = %q", got)
	}
}

func TestLookupIndic(t *testing.T) {
	// A code point below the first range, above the last, exact hits, and gaps.
	cat, pos := lookupIndic(0x0915) // ka
	if cat != catC || pos != posEnd {
		t.Errorf("0x0915 -> cat %d pos %d", cat, pos)
	}
	if cat, pos := lookupIndic(0x093F); cat != catM || pos != posPreM {
		t.Errorf("0x093F -> cat %d pos %d want matra/preM", cat, pos)
	}
	if cat, _ := lookupIndic(0x0000); cat != catX { // below all ranges
		t.Errorf("0x0000 -> cat %d want X", cat)
	}
	if cat, _ := lookupIndic(0x10FFFF); cat != catX { // above all ranges
		t.Errorf("0x10FFFF -> cat %d want X", cat)
	}
	if cat, _ := lookupIndic(0x0891); cat != catX { // gap before the Indic blocks
		t.Errorf("0x0891 -> cat %d want X", cat)
	}
}

func TestIndicCat(t *testing.T) {
	cases := []struct {
		r   rune
		cat uint8
	}{
		{0x200C, catZWNJ},
		{0x200D, catZWJ},
		{0x25CC, catDottedCircle},
		{0x0915, catC}, // no override
	}
	for _, c := range cases {
		if got, _ := indicCat(c.r); got != c.cat {
			t.Errorf("indicCat(U+%04X) = %d, want %d", c.r, got, c.cat)
		}
	}
}

func TestIsIndicConsonant(t *testing.T) {
	if !isIndicConsonant(catC) || !isIndicConsonant(catCS) {
		t.Error("catC/catCS should be consonants")
	}
	if isIndicConsonant(catM) || isIndicConsonant(catV) {
		t.Error("matra/vowel should not be consonants")
	}
}

func TestIsIndicStarter(t *testing.T) {
	for _, c := range []uint8{catC, catCS, catV, catRepha, catPlaceholder, catDottedCircle} {
		if !isIndicStarter(c) {
			t.Errorf("cat %d should be a starter", c)
		}
	}
	for _, c := range []uint8{catM, catH, catN, catSM, catX} {
		if isIndicStarter(c) {
			t.Errorf("cat %d should not be a starter", c)
		}
	}
}

func TestSegmentIndic(t *testing.T) {
	if got := segmentIndic(nil); got != nil {
		t.Errorf("segmentIndic(nil) = %v, want nil", got)
	}
	// "ककि a": ka | ka + i-matra | space+a. The virama case: "क्क" is one span
	// (halant holds the second consonant in the same syllable).
	cases := []struct {
		in   string
		want [][2]int
	}{
		{"क", [][2]int{{0, 1}}},
		{"कक", [][2]int{{0, 1}, {1, 2}}},  // starter break
		{"कि", [][2]int{{0, 2}}},          // matra attaches
		{"क्क", [][2]int{{0, 3}}},         // halant holds the conjunct together
		{"क a", [][2]int{{0, 1}, {1, 3}}}, // Indic then non-Indic run
		{"ab", [][2]int{{0, 2}}},          // pure non-Indic
		{"aक", [][2]int{{0, 1}, {1, 2}}},  // non-Indic then Indic starter
	}
	for _, c := range cases {
		if got := segmentIndic([]rune(c.in)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("segmentIndic(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestAfterBasePos(t *testing.T) {
	cases := []struct {
		cat, raw uint8
		want     uint8
	}{
		{catM, posEnd, posPostC}, // matra with no positional info
		{catM, posPreM, posPreM}, // matra keeps its raw position
		{catM, posAboveC, posAboveC},
		{catSM, posEnd, posSMVD},
		{catA, posEnd, posSMVD},
		{catH, posEnd, posBelowC},
		{catN, posEnd, posAboveC},
		{catC, posEnd, posBelowC},
		{catCS, posEnd, posBelowC},
		{catCM, posEnd, posBelowC},
		{catX, posEnd, posEnd}, // default
	}
	for _, c := range cases {
		if got := afterBasePos(c.cat, c.raw); got != c.want {
			t.Errorf("afterBasePos(%d,%d) = %d, want %d", c.cat, c.raw, got, c.want)
		}
	}
}

func TestIsIndicBase(t *testing.T) {
	for _, c := range []uint8{catV, catPlaceholder, catDottedCircle} {
		if !isIndicBase(c) {
			t.Errorf("cat %d should be a base fallback", c)
		}
	}
	for _, c := range []uint8{catC, catM, catH, catX} {
		if isIndicBase(c) {
			t.Errorf("cat %d should not be a base fallback", c)
		}
	}
}

func TestDecomposeIndic(t *testing.T) {
	// A Bengali split O sign decomposes into its pre-base and post-base parts,
	// both carrying the source cluster; a plain consonant is carried through.
	runes := []rune("বো") // U+09AC (ba) + U+09CB (vowel sign O -> 09C7 09BE)
	out, cl := decomposeIndic(runes)
	wantR := []rune{0x09AC, 0x09C7, 0x09BE}
	if !reflect.DeepEqual(out, wantR) {
		t.Errorf("decomposeIndic runes = %U, want %U", out, wantR)
	}
	if want := []int{0, 1, 1}; !reflect.DeepEqual(cl, want) {
		t.Errorf("decomposeIndic clusters = %v, want %v", cl, want)
	}
	// A three-part Kannada sign decomposes fully in one pass.
	out, _ = decomposeIndic([]rune{0x0CCB})
	if want := []rune{0x0CC6, 0x0CC2, 0x0CD5}; !reflect.DeepEqual(out, want) {
		t.Errorf("0CCB -> %U, want %U", out, want)
	}
	// A run with no split matra is unchanged.
	out, cl = decomposeIndic([]rune("क"))
	if !reflect.DeepEqual(out, []rune{0x0915}) || !reflect.DeepEqual(cl, []int{0}) {
		t.Errorf("plain decompose = %U,%v", out, cl)
	}
}

func TestFindIndicBase(t *testing.T) {
	C, H, Z := catC, catH, catZWJ
	// BASE_POS_LAST: the last consonant; -1 when there is none.
	if got := findIndicBase([]uint8{C, H, C}, 0, 3, basePosLast); got != 2 {
		t.Errorf("basePosLast last consonant = %d, want 2", got)
	}
	if got := findIndicBase([]uint8{catM, catM}, 0, 2, basePosLast); got != -1 {
		t.Errorf("basePosLast no consonant = %d, want -1", got)
	}
	// BASE_POS_LAST_SINHALA: the last consonant, but breaking before a
	// ZWJ-joined consonant; -1 when there is none.
	if got := findIndicBase([]uint8{C, C}, 0, 2, basePosLastSinhala); got != 1 {
		t.Errorf("sinhala last consonant = %d, want 1", got)
	}
	if got := findIndicBase([]uint8{C, Z, C}, 0, 3, basePosLastSinhala); got != 0 {
		t.Errorf("sinhala ZWJ break = %d, want 0", got)
	}
	if got := findIndicBase([]uint8{catM}, 0, 1, basePosLastSinhala); got != -1 {
		t.Errorf("sinhala no consonant = %d, want -1", got)
	}
}

func TestFirstCluster(t *testing.T) {
	if got := firstCluster(nil); got != 0 {
		t.Errorf("firstCluster(nil) = %d, want 0", got)
	}
	if got := firstCluster([]int{7, 8}); got != 7 {
		t.Errorf("firstCluster([7,8]) = %d, want 7", got)
	}
}

func TestGlyphsChanged(t *testing.T) {
	a := []opentype.GlyphIndex{1, 2, 3}
	if glyphsChanged(a, []opentype.GlyphIndex{1, 2, 3}) {
		t.Error("equal runs reported changed")
	}
	if !glyphsChanged(a, []opentype.GlyphIndex{1, 2}) {
		t.Error("length change not reported")
	}
	if !glyphsChanged(a, []opentype.GlyphIndex{1, 9, 3}) {
		t.Error("content change not reported")
	}
}

func TestAnalyzeIndic(t *testing.T) {
	deva := indicConfigs["deva"]

	// Non-Indic span: all positions posEnd, not indic.
	_, _, syl, indic := analyzeIndic([]rune("ab"), []int{0, 1}, deva)
	if indic || !reflect.DeepEqual(syl.poss, []uint8{posEnd, posEnd}) {
		t.Errorf("non-indic analyze = %+v,%v", syl, indic)
	}

	// Reph + base: the two reph glyphs park at the front (posRaToReph).
	_, _, syl, indic = analyzeIndic([]rune("र्क"), []int{0, 1, 2}, deva)
	if !syl.reph || !indic {
		t.Fatalf("र्क reph=%v indic=%v", syl.reph, indic)
	}
	if want := []uint8{posRaToReph, posRaToReph, posBaseC}; !reflect.DeepEqual(syl.poss, want) {
		t.Errorf("र्क poss = %v, want %v", syl.poss, want)
	}

	// Pre-base consonant + base: क्क -> [preC, preC, baseC], no reph.
	_, _, syl, _ = analyzeIndic([]rune("क्क"), []int{0, 1, 2}, deva)
	if syl.reph {
		t.Error("क्क should have no reph")
	}
	if want := []uint8{posPreC, posPreC, posBaseC}; !reflect.DeepEqual(syl.poss, want) {
		t.Errorf("क्क poss = %v, want %v", syl.poss, want)
	}

	// ka + i-matra: base at 0, matra pre-base.
	_, _, syl, _ = analyzeIndic([]rune("कि"), []int{0, 1}, deva)
	if want := []uint8{posBaseC, posPreM}; !reflect.DeepEqual(syl.poss, want) {
		t.Errorf("कि poss = %v, want %v", syl.poss, want)
	}

	// Leading Ra+virama but no following consonant: not a reph; Ra is the base.
	_, _, syl, _ = analyzeIndic([]rune("र्ा"), []int{0, 1, 2}, deva)
	if syl.reph {
		t.Errorf("Ra+virama+matra should not be a reph: %+v", syl)
	}

	// Independent vowel syllable: the vowel is the base.
	_, _, syl, indic = analyzeIndic([]rune("अ"), []int{0}, deva) // U+0905
	if !indic || syl.base != 0 || syl.poss[0] != posBaseC {
		t.Errorf("vowel syllable = %+v indic=%v", syl, indic)
	}

	// Defective cluster (a lone matra, no base): a dotted circle is inserted at
	// the front to carry the mark and becomes the base.
	lr, lc, syl, indic := analyzeIndic([]rune("ा"), []int{5}, deva) // U+093E AA matra
	if !indic || len(lr) != 2 || lr[0] != 0x25CC {
		t.Fatalf("defective analyze lr=%U indic=%v", lr, indic)
	}
	if lc[0] != 5 || syl.base != 0 || syl.poss[0] != posBaseC {
		t.Errorf("defective cluster = lc%v %+v", lc, syl)
	}

	// Pre-base-reordering Ra (Malayalam): ka + virama + Ra -> base is ka, the Ra
	// is the pref index.
	mlym := indicConfigs["mlym"]
	_, _, syl, _ = analyzeIndic([]rune{0x0D15, 0x0D4D, 0x0D30}, []int{0, 1, 2}, mlym)
	if syl.base != 0 || syl.prefIdx != 2 {
		t.Errorf("mlym pref analyze base=%d prefIdx=%d, want 0,2", syl.base, syl.prefIdx)
	}
}

func TestGposFeaturesIndic(t *testing.T) {
	got := gposFeatures("deva", nil)
	want := []string{"kern", "dist", "abvm", "blwm", "mark", "mkmk"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("indic gpos features = %v, want %v", got, want)
	}
}

func TestIsIndicTag(t *testing.T) {
	if !isIndicTag("deva") || isIndicTag("dev2") || isIndicTag("arab") {
		t.Error("isIndicTag should accept only canonical tags")
	}
}

func TestResolveScriptIndic(t *testing.T) {
	cases := []struct {
		want, runes, out string
	}{
		{"", "कि", "deva"},      // auto-detect Devanagari
		{"", "অ", "beng"},       // auto-detect Bengali
		{"deva", "abc", "deva"}, // explicit old tag
		{"dev2", "abc", "deva"}, // explicit v2 tag normalizes to canonical
		{"beng", "कि", "beng"},  // explicit overrides detection
	}
	for _, c := range cases {
		if got := resolveScript(c.want, []rune(c.runes)); got != c.out {
			t.Errorf("resolveScript(%q,%q) = %q, want %q", c.want, c.runes, got, c.out)
		}
	}
}

// --- synthetic Indic fonts --------------------------------------------------
//
// The published fonts module ships a real Noto Sans Devanagari but no Bengali,
// Tamil or Malayalam face, so the split-matra and pref/rphf feature-driven
// paths — which need a font whose GSUB fires (or deliberately does not) on
// chosen glyphs — are exercised with in-memory fonts assembled over the real
// Unicode Indic code points. The GSUB/font builders live in build_test.go and
// synth_test.go.

// gsubOneFeature wraps a lookup list under a single DFLT-script feature.
func gsubOneFeature(tag string, lookups [][]byte, idxs []uint16) []byte {
	scripts := []tScript{{tag: "DFLT", def: &tLangSys{required: 0xFFFF, features: []uint16{0}}}}
	feats := []tFeature{{tag: tag, lookups: idxs}}
	return buildLayoutTable(scripts, feats, lookups)
}

// indicSynthFont assembles a bare Indic font (500-unit advances) carrying the
// given cmap and GSUB table.
func indicSynthFont(t *testing.T, cmap map[rune]uint16, numGlyphs int, gsub []byte) *opentype.Font {
	t.Helper()
	adv := make([]int, numGlyphs)
	lsb := make([]int, numGlyphs)
	for i := range adv {
		adv[i] = 500
	}
	tables := baseTables(cmap, numGlyphs, adv, lsb)
	tables["GSUB"] = gsub
	f, err := opentype.Parse(assemble(tables))
	if err != nil {
		t.Fatalf("parse synthetic Indic font: %v", err)
	}
	return f
}

// TestIndicSplitMatra shapes Tamil ka + vowel sign O (U+0B95 U+0BCA). The O sign
// decomposes into a pre-base part (U+0BC6) and a post-base part (U+0BBE): the
// run must grow to three glyphs and the pre-base part must reorder before the
// base consonant.
func TestIndicSplitMatra(t *testing.T) {
	const ka, oSign, preV, postV = 0x0B95, 0x0BCA, 0x0BC6, 0x0BBE
	cmap := map[rune]uint16{ka: 1, preV: 2, postV: 3}
	f := indicSynthFont(t, cmap, 4, gsubOneFeature("pres", [][]byte{
		buildLookup(1, [][]byte{buildSingle1(0, buildCoverage1(90))}), // inert no-op
	}, []uint16{0}))
	got := Shape(f.NewFace(64), string([]rune{ka, oSign}), Options{Script: "tml2"})
	if len(got) != 3 {
		t.Fatalf("split matra not decomposed: %d glyphs %+v", len(got), got)
	}
	// The pre-base part derives from the O sign (source cluster 1) and must sit
	// before the base ka (cluster 0).
	pre := visualIndexOfCluster(got, 1)
	base := visualIndexOfCluster(got, 0)
	if pre < 0 || base < 0 || pre >= base {
		t.Errorf("split pre-base part not before base: pre@%d base@%d %+v", pre, base, got)
	}
}

// TestIndicPrefFired shapes Malayalam ka + virama + Ra (U+0D15 U+0D4D U+0D30)
// with a font whose pref feature substitutes the Ra: the pre-base-reordering Ra
// must move ahead of the base ka.
func TestIndicPrefFired(t *testing.T) {
	const ka, virama, ra = 0x0D15, 0x0D4D, 0x0D30
	cmap := map[rune]uint16{ka: 1, virama: 2, ra: 3}
	// pref: Ra (glyph 3) -> pre-base Ra form (glyph 4).
	pref := buildLookup(1, [][]byte{buildSingle1(1, buildCoverage1(3))})
	f := indicSynthFont(t, cmap, 5, gsubOneFeature("pref", [][]byte{pref}, []uint16{0}))
	got := Shape(f.NewFace(64), string([]rune{ka, virama, ra}), Options{Script: "mlm2"})
	ri := visualIndexOfCluster(got, 2) // the Ra (source cluster 2)
	ki := visualIndexOfCluster(got, 0) // the base ka
	if ri < 0 || ki < 0 || ri >= ki {
		t.Errorf("pref Ra not reordered before base: ra@%d ka@%d %+v", ri, ki, got)
	}
	if got[ri].GID != 4 {
		t.Errorf("pref did not substitute the Ra: GID %d, want 4 %+v", got[ri].GID, got)
	}
}

// TestIndicPrefNotFired shapes the same Malayalam cluster with a font that has
// no pref feature: the Ra stays a post-base consonant, after the base.
func TestIndicPrefNotFired(t *testing.T) {
	const ka, virama, ra = 0x0D15, 0x0D4D, 0x0D30
	cmap := map[rune]uint16{ka: 1, virama: 2, ra: 3}
	f := indicSynthFont(t, cmap, 4, gsubOneFeature("pres", [][]byte{
		buildLookup(1, [][]byte{buildSingle1(0, buildCoverage1(90))}),
	}, []uint16{0}))
	got := Shape(f.NewFace(64), string([]rune{ka, virama, ra}), Options{Script: "mlm2"})
	ri := visualIndexOfCluster(got, 2)
	ki := visualIndexOfCluster(got, 0)
	if ri < 0 || ki < 0 || ri <= ki {
		t.Errorf("pref not fired but Ra moved before base: ra@%d ka@%d %+v", ri, ki, got)
	}
}

// TestIndicRephNotFired shapes Devanagari Ra + virama + ka with a font that has
// no rphf feature: the leading Ra is not turned into a reph, so it stays at the
// front (before the base) rather than reordering to the per-script reph slot.
func TestIndicRephNotFired(t *testing.T) {
	const ra, virama, ka = 0x0930, 0x094D, 0x0915
	cmap := map[rune]uint16{ra: 1, virama: 2, ka: 3}
	f := indicSynthFont(t, cmap, 4, gsubOneFeature("pres", [][]byte{
		buildLookup(1, [][]byte{buildSingle1(0, buildCoverage1(90))}),
	}, []uint16{0}))
	got := Shape(f.NewFace(64), string([]rune{ra, virama, ka}), Options{Script: "dev2"})
	reph := visualIndexOfCluster(got, 0) // the Ra
	base := visualIndexOfCluster(got, 2) // the base ka
	if reph < 0 || base < 0 || reph >= base {
		t.Errorf("un-fired reph not kept before base: reph@%d base@%d %+v", reph, base, got)
	}
}

// TestIndicDottedCircle shapes a lone Devanagari matra (U+093E) with the real
// Noto Sans Devanagari font: a dotted circle (U+25CC, glyph 789) is inserted as
// the base and precedes the matra.
func TestIndicDottedCircle(t *testing.T) {
	f := notoDevanagari(t)
	got := Shape(f.NewFace(64), "ा", Options{Script: "dev2"})
	if len(got) < 2 {
		t.Fatalf("lone matra not padded with a dotted circle: %+v", got)
	}
	dc, _ := f.GlyphIndex(0x25CC)
	if got[0].GID != dc {
		t.Errorf("first glyph = %d, want dotted circle %d: %+v", got[0].GID, dc, got)
	}
}

// TestIndicSinhalaRouting confirms Sinhala routes to the Indic shaper (its
// dedicated model) rather than USE, both when auto-detected and when tagged.
func TestIndicSinhalaRouting(t *testing.T) {
	if got := resolveScript("", []rune("ක")); got != "sinh" {
		t.Errorf("auto-detect Sinhala = %q, want sinh", got)
	}
	if got := resolveScript("sinh", nil); got != "sinh" {
		t.Errorf("explicit sinh = %q, want sinh", got)
	}
	if c, ok := indicConfigForTag("sinh"); !ok || c.basePos != basePosLastSinhala {
		t.Errorf("sinh config = %+v,%v want Sinhala base model", c, ok)
	}
}
