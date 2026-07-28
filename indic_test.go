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

func TestAnalyzeSyllable(t *testing.T) {
	deva := indicConfigs["deva"]

	// Non-Indic span: all positions posEnd, not indic.
	pos, reph, indic := analyzeSyllable([]rune("ab"), 0, 2, deva)
	if indic || reph || !reflect.DeepEqual(pos, []uint8{posEnd, posEnd}) {
		t.Errorf("non-indic analyze = %v,%v,%v", pos, reph, indic)
	}

	// Reph + base: र्क -> [rephPos, rephPos, baseC].
	pos, reph, indic = analyzeSyllable([]rune("र्क"), 0, 3, deva)
	if !reph || !indic {
		t.Fatalf("र्क reph=%v indic=%v", reph, indic)
	}
	if want := []uint8{deva.rephPos, deva.rephPos, posBaseC}; !reflect.DeepEqual(pos, want) {
		t.Errorf("र्क pos = %v, want %v", pos, want)
	}

	// Pre-base consonant + base: क्क -> [preC(ka), preC(virama), baseC].
	pos, reph, _ = analyzeSyllable([]rune("क्क"), 0, 3, deva)
	if reph {
		t.Error("क्क should have no reph")
	}
	if want := []uint8{posPreC, posPreC, posBaseC}; !reflect.DeepEqual(pos, want) {
		t.Errorf("क्क pos = %v, want %v", pos, want)
	}

	// ka + i-matra: base at 0, matra pre-base.
	pos, _, _ = analyzeSyllable([]rune("कि"), 0, 2, deva)
	if want := []uint8{posBaseC, posPreM}; !reflect.DeepEqual(pos, want) {
		t.Errorf("कि pos = %v, want %v", pos, want)
	}

	// Leading Ra+virama but no following consonant (Ra+virama+matra): not a
	// reph; the Ra itself becomes the base.
	pos, reph, _ = analyzeSyllable([]rune("र्ा"), 0, 3, deva) // Ra, virama, AA-matra
	if reph {
		t.Errorf("Ra+virama+matra should not be a reph: %v", pos)
	}

	// Independent vowel syllable: base is the vowel.
	pos, _, indic = analyzeSyllable([]rune("अ"), 0, 1, deva) // U+0905
	if !indic || !reflect.DeepEqual(pos, []uint8{posBaseC}) {
		t.Errorf("vowel syllable pos = %v indic=%v", pos, indic)
	}

	// Indic but no consonant and no vowel: a lone matra. base defaults to 0.
	pos, _, indic = analyzeSyllable([]rune("ा"), 0, 1, deva) // U+093E AA matra
	if !indic || pos[0] != posBaseC {
		t.Errorf("lone matra pos = %v indic=%v", pos, indic)
	}
}

func TestReorderByPos(t *testing.T) {
	gl := []opentype.GlyphIndex{10, 20, 30}
	cl := []int{0, 1, 2}
	// pos: base(4) for cluster0, preM(2) for cluster1, smvd(13) for cluster2.
	posOf := map[int]uint8{0: posBaseC, 1: posPreM, 2: posSMVD}
	og, oc := reorderByPos(gl, cl, posOf)
	if !reflect.DeepEqual(og, []opentype.GlyphIndex{20, 10, 30}) {
		t.Errorf("reordered glyphs = %v", og)
	}
	if !reflect.DeepEqual(oc, []int{1, 0, 2}) {
		t.Errorf("reordered clusters = %v", oc)
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
