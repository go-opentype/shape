// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"os"
	"reflect"
	"testing"

	"github.com/go-opentype/fonts/notosansthai"
	"github.com/go-opentype/opentype"
)

// --- classification & dispatch -------------------------------------------------

func TestUseClass(t *testing.T) {
	cases := []struct {
		r   rune
		cat useCat
	}{
		{0x0E01, ucB},     // THAI KO KAI
		{0x0E35, ucVAbv},  // THAI SARA II (above vowel)
		{0x0E48, ucVMAbv}, // THAI MAI EK (tone mark, above)
		{0x0E38, ucVBlw},  // THAI SARA U (below vowel)
		{0x0000, ucO},     // below every range
		{0x0041, ucO},     // 'A', in a gap between ranges
		{0x10FFFF, ucO},   // above every range
	}
	for _, c := range cases {
		if got := useClass(c.r); got != c.cat {
			t.Errorf("useClass(U+%04X) = %d, want %d", c.r, got, c.cat)
		}
	}
}

func TestIsUSERuneAndHasUSE(t *testing.T) {
	if !isUSERune(0x0E01) { // Thai
		t.Error("isUSERune(Thai) = false, want true")
	}
	for _, r := range []rune{0x0000, 0x0041, 0x0628 /*Arabic*/, 0x10FFFF} {
		if isUSERune(r) {
			t.Errorf("isUSERune(U+%04X) = true, want false", r)
		}
	}
	if !hasUSE([]rune("กา")) {
		t.Error("hasUSE(Thai) = false, want true")
	}
	if hasUSE([]rune("abc")) {
		t.Error("hasUSE(latin) = true, want false")
	}
}

func TestResolveScriptUSE(t *testing.T) {
	cases := []struct {
		want, runes, out string
	}{
		{"thai", "abc", "use"},  // explicit USE tag
		{"LANA", "abc", "use"},  // case-insensitive USE tag
		{"", "กา", "use"},       // auto-detect Thai -> USE
		{"khmr", "", "use"},     // explicit Khmer
		{"cyrl", "abc", "dflt"}, // non-USE, non-Arabic tag
	}
	for _, c := range cases {
		if got := resolveScript(c.want, []rune(c.runes)); got != c.out {
			t.Errorf("resolveScript(%q, %q) = %q, want %q", c.want, c.runes, got, c.out)
		}
	}
}

func TestGposFeaturesUSE(t *testing.T) {
	want := []string{"kern", "dist", "abvm", "blwm", "mark", "mkmk", "ss01"}
	if got := gposFeatures("use", []string{"ss01"}); !reflect.DeepEqual(got, want) {
		t.Errorf("gposFeatures(use) = %v, want %v", got, want)
	}
}

// --- USE category derivation ---------------------------------------------------

func TestDeriveUSECategory(t *testing.T) {
	cases := []struct {
		uisc, uipc, ugc, want string
	}{
		// Base clauses (first switch).
		{"Consonant", "", "Lo", "B"},
		{"Number", "", "Nd", "B"},
		{"Consonant_Head_Letter", "", "Lo", "B"},
		{"Tone_Letter", "", "Lo", "B"},
		{"Vowel_Independent", "", "Lo", "B"},
		{"Avagraha", "", "Lo", "B"},
		{"Avagraha", "", "Mn", "O"}, // non-Lo, unmatched later -> O
		{"Bindu", "", "Lo", "B"},
		{"Consonant_Final", "", "Lo", "B"},
		{"Consonant_Medial", "", "Lo", "B"},
		{"Consonant_Subjoined", "", "Lo", "B"},
		{"Vowel", "", "Lo", "B"},
		{"Vowel_Dependent", "", "Lo", "B"},
		// Consonant modifiers (position via useModPos).
		{"Nukta", "Top", "Mn", "CMAbv"},
		{"Gemination_Mark", "Bottom", "Mn", "CMBlw"},
		{"Consonant_Killer", "", "Mn", "CM"},
		{"Symbol_Modifier", "", "Mn", "CM"},
		// Stacker, finals (position via useFinalPos), final modifier.
		{"Consonant_With_Stacker", "", "Lo", "CS"},
		{"Consonant_Final", "Top", "Mn", "FAbv"},
		{"Consonant_Succeeding_Repha", "Bottom", "Mn", "FBlw"},
		{"Consonant_Final", "Right", "Mn", "FPst"},
		{"Consonant_Final", "", "Mn", "F"},
		{"Syllable_Modifier", "", "Mn", "FM"},
		// Placeholders, halants, number joiner, independents.
		{"Consonant_Placeholder", "", "So", "GB"},
		{"Virama", "", "Mn", "H"},
		{"Invisible_Stacker", "", "Mn", "H"},
		{"Number_Joiner", "", "Mn", "HN"},
		{"Consonant_Dead", "", "Lo", "IND"},
		{"Modifying_Letter", "", "Lo", "IND"},
		{"Other", "", "Lo", "IND"},
		// Medials (position via useMedialPos).
		{"Consonant_Medial", "Left", "Mn", "MPre"},
		{"Consonant_Medial", "Top", "Mn", "MAbv"},
		{"Consonant_Medial", "Bottom", "Mn", "MBlw"},
		{"Consonant_Medial", "Right", "Mn", "MPst"},
		{"Consonant_Medial", "", "Mn", "M"},
		// Numbers, repha, reordering killer, subjoined.
		{"Brahmi_Joining_Number", "", "Nd", "N"},
		{"Consonant_Preceding_Repha", "", "Lo", "R"},
		{"Consonant_Prefixed", "", "Lo", "R"},
		{"Reordering_Killer", "", "Mn", "RK"},
		{"Consonant_Subjoined", "", "Mn", "SUB"},
		// Vowels (all useVowelPos branches) and pure killer.
		{"Vowel_Dependent", "Left", "Mn", "VPre"},
		{"Vowel_Dependent", "Top", "Mn", "VAbv"},
		{"Vowel_Dependent", "Bottom", "Mn", "VBlw"},
		{"Vowel_Dependent", "Right", "Mn", "VPst"},
		{"Vowel_Dependent", "Overstruck", "Mn", "VBlw"},
		{"Vowel_Dependent", "Top_And_Left", "Mn", "VPre"},
		{"Vowel", "Weird", "Mn", "V"}, // useVowelPos default
		{"Pure_Killer", "", "Mn", "V"},
		// Vowel modifiers.
		{"Tone_Mark", "Top", "Mn", "VMAbv"},
		{"Bindu", "Top", "Mn", "VMAbv"},
		{"Cantillation_Mark", "Bottom", "Mn", "VMBlw"},
		{"Register_Shifter", "Left", "Mn", "VMPre"},
		{"Visarga", "Right", "Mn", "VMPst"},
		// Joiners, punctuation fallback, other.
		{"Joiner", "", "Cf", "ZWJ"},
		{"Non_Joiner", "", "Cf", "ZWNJ"},
		{"Nonsense", "", "Po", "IND"}, // ugc==Po fallback
		{"Nonsense", "", "Ll", "O"},   // default
	}
	for _, c := range cases {
		if got := DeriveUSECategory(c.uisc, c.uipc, c.ugc); got != c.want {
			t.Errorf("DeriveUSECategory(%q,%q,%q) = %q, want %q", c.uisc, c.uipc, c.ugc, got, c.want)
		}
	}
}

// --- syllable segmentation -----------------------------------------------------

func TestUseSegment(t *testing.T) {
	cases := []struct {
		name string
		cats []useCat
		want [][2]int
	}{
		{"empty", nil, nil},
		{"standard base+aboveVowel", []useCat{ucB, ucVAbv}, [][2]int{{0, 2}}},
		{"repha+base", []useCat{ucR, ucB, ucVAbv}, [][2]int{{0, 3}}},
		{"stacker+base", []useCat{ucCS, ucB}, [][2]int{{0, 2}}},
		{"repha without base", []useCat{ucR, ucO}, [][2]int{{0, 1}, {1, 2}}},
		{"base+VS+mods", []useCat{ucB, ucVS, ucCMAbv, ucCMBlw}, [][2]int{{0, 4}}},
		{"H base repeat", []useCat{ucB, ucH, ucB, ucVPst}, [][2]int{{0, 4}}},
		{"H base with VS", []useCat{ucB, ucH, ucB, ucVS, ucVPst}, [][2]int{{0, 5}}},
		{"SUB", []useCat{ucB, ucSUB, ucCMAbv}, [][2]int{{0, 3}}},
		{"virama terminated", []useCat{ucB, ucH}, [][2]int{{0, 2}}},
		{"reordering killer terminated", []useCat{ucB, ucRK}, [][2]int{{0, 2}}},
		{"sakot joins two bases", []useCat{ucB, ucSk, ucB, ucVPst}, [][2]int{{0, 4}}},
		{"sakot terminated", []useCat{ucB, ucSk}, [][2]int{{0, 2}}},
		{"sakot then non-base", []useCat{ucB, ucSk, ucO}, [][2]int{{0, 2}, {2, 3}}},
		{"halant ZWJ base joins", []useCat{ucB, ucH, ucZWJ, ucB}, [][2]int{{0, 4}}},
		{"sakot ZWNJ base joins", []useCat{ucB, ucSk, ucZWNJ, ucB, ucVS}, [][2]int{{0, 5}}},
		{"broken leading marks", []useCat{ucVAbv, ucVBlw}, [][2]int{{0, 2}}},
		{"broken lead with VS", []useCat{ucVMAbv, ucVS}, [][2]int{{0, 2}}},
		{"broken then base", []useCat{ucVAbv, ucB}, [][2]int{{0, 1}, {1, 2}}},
		{"full tail", []useCat{ucB, ucMPre, ucMAbv, ucMBlw, ucMPst, ucVPre, ucVMAbv, ucFAbv, ucFM}, [][2]int{{0, 9}}},
		{"number joiner", []useCat{ucN, ucHN, ucN, ucHN}, [][2]int{{0, 4}}},
		{"number joiner with VS", []useCat{ucN, ucHN, ucN, ucVS}, [][2]int{{0, 4}}},
		{"number+VS", []useCat{ucN, ucVS}, [][2]int{{0, 2}}},
		{"symbol", []useCat{ucS, ucVS, ucCMAbv}, [][2]int{{0, 3}}},
		{"independent+VS", []useCat{ucIND, ucVS}, [][2]int{{0, 2}}},
		{"joiner absorb", []useCat{ucB, ucVAbv, ucZWJ, ucB}, [][2]int{{0, 3}, {3, 4}}},
		{"two clusters", []useCat{ucB, ucB}, [][2]int{{0, 1}, {1, 2}}},
	}
	for _, c := range cases {
		if got := useSegment(c.cats); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: useSegment(%v) = %v, want %v", c.name, c.cats, got, c.want)
		}
	}
}

func TestUseSyllableIDs(t *testing.T) {
	// [B VAbv][B] -> ids 0,0,1
	got := useSyllableIDs([]useCat{ucB, ucVAbv, ucB})
	if want := []int{0, 0, 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("useSyllableIDs = %v, want %v", got, want)
	}
}

// --- reordering ----------------------------------------------------------------

func TestReorderRange(t *testing.T) {
	// falses returns a zeroed flag slice of length n.
	falses := func(n int) []bool { return make([]bool, n) }
	cases := []struct {
		gc         []useCat
		pref, rphf []bool
		want       []int
	}{
		{[]useCat{ucB, ucVAbv}, falses(2), falses(2), []int{0, 1}}, // no pre-base movement
		{[]useCat{ucB, ucVPre}, falses(2), falses(2), []int{1, 0}}, // VPre before base
		{[]useCat{ucVMPre, ucVPre, ucB}, falses(3), falses(3), []int{0, 1, 2}},
		{[]useCat{ucB, ucVPre, ucVMPre}, falses(3), falses(3), []int{2, 1, 0}}, // VMPre then VPre then base
		// pref-fired consonant (index 1) moves before the base (index 0).
		{[]useCat{ucB, ucB}, []bool{false, true}, falses(2), []int{1, 0}},
		// A pre-base vowel precedes a pref consonant: VPre(2), pref(1), base(0).
		{[]useCat{ucB, ucB, ucVPre}, []bool{false, true, false}, falses(3), []int{2, 1, 0}},
		// rphf-fired head (index 0) reorders after the base (index 1).
		{[]useCat{ucB, ucB}, falses(2), []bool{true, false}, []int{1, 0}},
	}
	for _, c := range cases {
		if got := reorderRange(c.gc, c.pref, c.rphf, 0, len(c.gc)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("reorderRange(%v,pref=%v,rphf=%v) = %v, want %v", c.gc, c.pref, c.rphf, got, c.want)
		}
	}
}

func TestRephaReorder(t *testing.T) {
	cases := []struct {
		gc   []useCat
		rphf []bool
		rest []int
		want []int
	}{
		{[]useCat{ucR, ucB, ucVAbv}, []bool{false, false, false}, []int{0, 1, 2}, []int{1, 0, 2}}, // static R after base
		{[]useCat{ucR, ucGB}, []bool{false, false}, []int{0, 1}, []int{1, 0}},                     // base can be GB
		{[]useCat{ucB, ucB}, []bool{true, false}, []int{0, 1}, []int{1, 0}},                       // rphf-fired head after base
		{[]useCat{ucR}, []bool{false}, []int{0}, []int{0}},                                        // R with no base
		{[]useCat{ucB}, []bool{false}, []int{0}, []int{0}},                                        // not a repha
		{nil, nil, nil, nil}, // empty
	}
	for _, c := range cases {
		if got := rephaReorder(c.gc, c.rphf, c.rest); !reflect.DeepEqual(got, c.want) {
			t.Errorf("rephaReorder(%v,%v,%v) = %v, want %v", c.gc, c.rphf, c.rest, got, c.want)
		}
	}
}

func TestUseReorder(t *testing.T) {
	falses := func(n int) []bool { return make([]bool, n) }
	// Empty run is returned unchanged.
	if r, c := useReorder(nil, nil, nil, nil, nil, nil); r != nil || c != nil {
		t.Errorf("useReorder(nil) = %v,%v want nil,nil", r, c)
	}
	// One syllable [B, VPre, VMPre] (positions 0,1,2) -> reordered VMPre,VPre,B.
	run := []opentype.GlyphIndex{10, 20, 30}
	pos := []int{0, 1, 2}
	cats := []useCat{ucB, ucVPre, ucVMPre}
	syll := []int{0, 0, 0}
	gotRun, gotPos := useReorder(run, pos, cats, syll, falses(3), falses(3))
	if want := []opentype.GlyphIndex{30, 20, 10}; !reflect.DeepEqual(gotRun, want) {
		t.Errorf("useReorder run = %v, want %v", gotRun, want)
	}
	if want := []int{2, 1, 0}; !reflect.DeepEqual(gotPos, want) {
		t.Errorf("useReorder pos = %v, want %v", gotPos, want)
	}
	// Two syllables reorder independently: [B,VPre | B] -> [VPre,B | B].
	run2 := []opentype.GlyphIndex{10, 20, 30}
	pos2 := []int{0, 1, 2}
	cats2 := []useCat{ucB, ucVPre, ucB}
	syll2 := []int{0, 0, 1}
	gr, _ := useReorder(run2, pos2, cats2, syll2, falses(3), falses(3))
	if want := []opentype.GlyphIndex{20, 10, 30}; !reflect.DeepEqual(gr, want) {
		t.Errorf("two-syllable reorder = %v, want %v", gr, want)
	}
	// A pref-fired glyph moves before the base: [B, B(pref)] -> [B(pref), B].
	run3 := []opentype.GlyphIndex{10, 20}
	gr3, _ := useReorder(run3, []int{0, 1}, []useCat{ucB, ucB}, []int{0, 0}, []bool{false, true}, falses(2))
	if want := []opentype.GlyphIndex{20, 10}; !reflect.DeepEqual(gr3, want) {
		t.Errorf("pref reorder = %v, want %v", gr3, want)
	}
}

// --- feature lists -------------------------------------------------------------

func TestUseFeatureLists(t *testing.T) {
	i := useStageI()
	if len(i) != 13 || i[0].Tag != "locl" || i[12].Tag != "cjct" {
		t.Errorf("useStageI = %+v", i)
	}
	ii := useStageII([]string{"ss01"})
	if len(ii) != 10 || ii[0].Tag != "isol" || ii[8].Tag != "psts" || ii[9].Tag != "ss01" {
		t.Errorf("useStageII = %+v", ii)
	}
}

// --- integration: synthetic (no layout tables) ---------------------------------

// TestUSEBareFont drives the USE path over a font without GSUB/GPOS: shapeUSE
// classifies and reorders the raw cmap glyphs, positions are zero, output is
// left-to-right.
func TestUSEBareFont(t *testing.T) {
	f := bareFont(t, map[rune]uint16{0x0E01: 1, 0x0E35: 2}, 1000, nil)
	if f.GSUB() != nil || f.GPOS() != nil {
		t.Fatal("bare font unexpectedly has layout tables")
	}
	got := Shape(f.NewFace(1000), "กี", Options{Script: "thai"})
	if len(got) != 2 {
		t.Fatalf("shaped %d glyphs, want 2: %+v", len(got), got)
	}
	if got[0].Cluster != 0 || got[1].Cluster != 1 {
		t.Errorf("clusters = %d,%d want 0,1", got[0].Cluster, got[1].Cluster)
	}
	for _, g := range got {
		if g.XOffset != 0 || g.YOffset != 0 {
			t.Errorf("bare USE font produced positioning: %+v", g)
		}
	}
}

// --- integration: real font (Noto Sans Thai) -----------------------------------

func notoThai(t *testing.T) *opentype.Font {
	t.Helper()
	f, err := opentype.Parse(notosansthai.TTF)
	if err != nil {
		t.Fatalf("parse Noto Sans Thai: %v", err)
	}
	return f
}

// TestRealThaiMarkStacked shapes KO KAI + SARA II (above vowel) + MAI EK (tone
// mark, above) through the USE path and asserts the tone mark is stacked above by
// GPOS: a non-zero YOffset. This proves the USE pipeline (classification,
// stage-I/stage-II GSUB, then GPOS mark/abvm positioning) runs on a real font.
func TestRealThaiMarkStacked(t *testing.T) {
	f := notoThai(t)
	face := f.NewFace(128)
	got := Shape(face, "กี่", Options{Script: "thai", Features: []string{"ss01"}})
	if len(got) < 3 {
		t.Fatalf("shaped %d glyphs, want >= 3: %+v", len(got), got)
	}
	// Every source rune is covered by some cluster.
	seen := map[int]bool{}
	for _, g := range got {
		if g.Cluster < 0 || g.Cluster > 2 {
			t.Errorf("cluster %d out of range: %+v", g.Cluster, g)
		}
		seen[g.Cluster] = true
	}
	for c := 0; c < 3; c++ {
		if !seen[c] {
			t.Errorf("source rune %d not covered", c)
		}
	}
	// The tone mark (cluster 2) must be lifted above the base by GPOS.
	stacked := false
	for _, g := range got {
		if g.Cluster == 2 && g.YOffset != 0 {
			stacked = true
		}
	}
	if !stacked {
		t.Errorf("tone mark (cluster 2) has no YOffset; mark not stacked: %+v", got)
	}
	t.Logf("Thai ก+ ี + ่ = %+v", got)
}

// TestRealThaiAutoDetect shapes Thai with no explicit script and asserts it is
// routed to the USE shaper (a base plus a positioned above vowel), matching the
// forced-script result.
func TestRealThaiAutoDetect(t *testing.T) {
	f := notoThai(t)
	face := f.NewFace(96)
	auto := Shape(face, "กี", Options{})
	forced := Shape(face, "กี", Options{Script: "thai"})
	if !reflect.DeepEqual(auto, forced) {
		t.Errorf("auto-detect (%+v) != forced (%+v)", auto, forced)
	}
	if len(auto) < 2 {
		t.Fatalf("shaped %d glyphs, want >= 2", len(auto))
	}
}

// --- split-vowel decomposition & dotted circle (synthetic) ---------------------

func TestDecomposeUSE(t *testing.T) {
	// A non-split run passes through unchanged, one source index per rune.
	dr, src := decomposeUSE([]rune{0x1B13, 0x0E01})
	if !reflect.DeepEqual(dr, []rune{0x1B13, 0x0E01}) || !reflect.DeepEqual(src, []int{0, 1}) {
		t.Errorf("passthrough: dr=%v src=%v", dr, src)
	}
	// Balinese U+1B40 splits into a pre-base part (VPre) and a post-base part
	// (VPst), both carrying the composite's source index (1).
	dr, src = decomposeUSE([]rune{0x1B13, 0x1B40})
	if !reflect.DeepEqual(dr, []rune{0x1B13, 0x1B3E, 0x1B35}) {
		t.Errorf("split runes = %v", dr)
	}
	if !reflect.DeepEqual(src, []int{0, 1, 1}) {
		t.Errorf("split src = %v", src)
	}
	// Tibetan U+0F76 splits into two parts.
	dr, _ = decomposeUSE([]rune{0x0F76})
	if !reflect.DeepEqual(dr, []rune{0x0FB2, 0x0F80}) {
		t.Errorf("tibetan split = %v", dr)
	}
}

func TestInsertDottedCircles(t *testing.T) {
	dc := useClass(0x25CC)
	// A defective cluster (a bare above-vowel) gets a leading dotted circle,
	// inheriting the mark's source index.
	dr := []rune{0x0E48}
	src := []int{0}
	cats := []useCat{ucVMAbv}
	ndr, nsrc, ncats := insertDottedCircles(dr, src, cats)
	if !reflect.DeepEqual(ndr, []rune{0x25CC, 0x0E48}) {
		t.Errorf("runes = %v", ndr)
	}
	if !reflect.DeepEqual(nsrc, []int{0, 0}) {
		t.Errorf("src = %v", nsrc)
	}
	if !reflect.DeepEqual(ncats, []useCat{dc, ucVMAbv}) {
		t.Errorf("cats = %v", ncats)
	}
	// A well-formed cluster is returned unchanged (same backing slices).
	dr2 := []rune{0x0E01, 0x0E48}
	src2 := []int{0, 1}
	cats2 := []useCat{ucB, ucVMAbv}
	ndr2, _, _ := insertDottedCircles(dr2, src2, cats2)
	if !reflect.DeepEqual(ndr2, dr2) {
		t.Errorf("non-defective changed: %v", ndr2)
	}
}

// --- real-font USE deferrals ---------------------------------------------------

// testFont loads a font from testdata, skipping nothing (the fonts are vendored
// with the tests so a missing file is a hard failure).
func testFont(t *testing.T, name string) *opentype.Font {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	f, err := opentype.Parse(b)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return f
}

// visualIndexOf returns the visual position of the first glyph with the given
// GID, or -1.
func visualIndexOf(gs []Glyph, gid opentype.GlyphIndex) int {
	for i, g := range gs {
		if g.GID == gid {
			return i
		}
	}
	return -1
}

// TestRealThaiDottedCircle shapes a lone Thai tone mark (a defective cluster with
// no base) and asserts a U+25CC dotted circle was inserted before it.
func TestRealThaiDottedCircle(t *testing.T) {
	f := notoThai(t)
	dc, _ := f.GlyphIndex(0x25CC)
	got := Shape(f.NewFace(128), string([]rune{0x0E48}), Options{Script: "thai"})
	if len(got) != 2 {
		t.Fatalf("lone tone mark -> %d glyphs, want 2: %+v", len(got), got)
	}
	if got[0].GID != dc {
		t.Errorf("first glyph gid=%d, want dotted circle %d: %+v", got[0].GID, dc, got)
	}
	// A well-formed base+tone cluster gets no dotted circle.
	ok := Shape(f.NewFace(128), string([]rune{0x0E01, 0x0E48}), Options{Script: "thai"})
	if visualIndexOf(ok, dc) >= 0 {
		t.Errorf("well-formed cluster got a dotted circle: %+v", ok)
	}
}

// TestRealKhmerPrefReorder shapes Khmer KA + COENG + RO: the pref feature ligates
// the coeng-ro into a pre-base form that must reorder ahead of the base KA.
func TestRealKhmerPrefReorder(t *testing.T) {
	f := testFont(t, "NotoSansKhmer-Regular.ttf")
	ka, _ := f.GlyphIndex(0x1780)
	got := Shape(f.NewFace(128), string([]rune{0x1780, 0x17D2, 0x179A}), Options{Script: "khmr"})
	if len(got) < 2 {
		t.Fatalf("shaped %d glyphs, want >= 2: %+v", len(got), got)
	}
	kaPos := visualIndexOf(got, ka)
	if kaPos <= 0 {
		t.Errorf("base KA at visual %d, want a pre-base glyph before it: %+v", kaPos, got)
	}
}

// TestRealKhmerPreBaseVowel shapes Khmer KA + SRA E (a pre-base VPre vowel stored
// after the base) and asserts the vowel reorders ahead of the base.
func TestRealKhmerPreBaseVowel(t *testing.T) {
	f := testFont(t, "NotoSansKhmer-Regular.ttf")
	ka, _ := f.GlyphIndex(0x1780)
	sraE, _ := f.GlyphIndex(0x17C1)
	got := Shape(f.NewFace(128), string([]rune{0x1780, 0x17C1}), Options{Script: "khmr"})
	if visualIndexOf(got, sraE) != 0 || visualIndexOf(got, ka) != 1 {
		t.Errorf("SRA E did not reorder before base KA: %+v", got)
	}
}

// TestRealMyanmarPreBaseVowel shapes Myanmar KA + U+1031 (a pre-base VPre vowel
// stored after the base) and asserts the vowel reorders ahead of the base.
func TestRealMyanmarPreBaseVowel(t *testing.T) {
	f := testFont(t, "NotoSansMyanmar-Regular.ttf")
	ka, _ := f.GlyphIndex(0x1000)
	e, _ := f.GlyphIndex(0x1031)
	got := Shape(f.NewFace(128), string([]rune{0x1000, 0x1031}), Options{Script: "mymr"})
	if visualIndexOf(got, e) != 0 || visualIndexOf(got, ka) != 1 {
		t.Errorf("Myanmar E did not reorder before base KA: %+v", got)
	}
}

// TestRealBalineseSplitVowel shapes Balinese KA + U+1B40 (a two-part vowel): it
// must decompose into a pre-base part (U+1B3E, VPre) reordered before the base
// and a post-base part (U+1B35, VPst) left after it.
func TestRealBalineseSplitVowel(t *testing.T) {
	f := testFont(t, "NotoSansBalinese-Regular.ttf")
	ka, _ := f.GlyphIndex(0x1B13)
	pre, _ := f.GlyphIndex(0x1B3E)
	post, _ := f.GlyphIndex(0x1B35)
	got := Shape(f.NewFace(128), string([]rune{0x1B13, 0x1B40}), Options{Script: "bali"})
	pi, ki, qi := visualIndexOf(got, pre), visualIndexOf(got, ka), visualIndexOf(got, post)
	if pi < 0 || ki < 0 || qi < 0 {
		t.Fatalf("split-vowel parts missing: pre=%d base=%d post=%d in %+v", pi, ki, qi, got)
	}
	if !(pi < ki && ki < qi) {
		t.Errorf("split vowel mis-ordered: pre=%d base=%d post=%d: %+v", pi, ki, qi, got)
	}
	// Both split parts carry the composite vowel's source cluster (index 1).
	for _, g := range got {
		if (g.GID == pre || g.GID == post) && g.Cluster != 1 {
			t.Errorf("split part gid=%d cluster=%d, want 1: %+v", g.GID, g.Cluster, got)
		}
	}
}

// useRphfFont assembles a minimal USE font whose rphf feature ligates a leading
// consonant + halant into a repha glyph. Khmer runes RO (U+179A), COENG (U+17D2,
// a halant) and KA (U+1780) map to glyphs 1, 2 and 3; the rphf ligature collapses
// RO+COENG (glyphs 1+2) into the repha glyph 4.
func useRphfFont(t *testing.T) *opentype.Font {
	t.Helper()
	numGlyphs := 5
	adv := []int{500, 500, 500, 500, 500}
	lsb := []int{0, 0, 0, 0, 0}
	tables := baseTables(map[rune]uint16{0x179A: 1, 0x17D2: 2, 0x1780: 3}, numGlyphs, adv, lsb)
	set := buildLigatureSet([][]byte{buildLigature(4, 2)}) // glyphs 1,2 -> 4
	lig := buildLookup(4, [][]byte{buildLigatureSubst(buildCoverage1(1), [][]byte{set})})
	scripts := []tScript{{tag: "DFLT", def: &tLangSys{required: 0xFFFF, features: []uint16{0}}}}
	feats := []tFeature{{tag: "rphf", lookups: []uint16{0}}}
	tables["GSUB"] = buildLayoutTable(scripts, feats, [][]byte{lig})
	f, err := opentype.Parse(assemble(tables))
	if err != nil {
		t.Fatalf("parse rphf font: %v", err)
	}
	return f
}

// TestUSERphfRepha shapes RO + COENG + KA through a font whose rphf feature forms
// a repha: the ligated head (a dynamic repha) must reorder after the base KA. It
// exercises feature-based repha detection and reordering deterministically.
func TestUSERphfRepha(t *testing.T) {
	f := useRphfFont(t)
	got := Shape(f.NewFace(1000), string([]rune{0x179A, 0x17D2, 0x1780}), Options{Script: "khmr"})
	if len(got) != 2 {
		t.Fatalf("rphf run -> %d glyphs, want 2 (repha + base): %+v", len(got), got)
	}
	// Base KA (glyph 3, source cluster 2) comes first; the repha (glyph 4, from
	// the leading RO, source cluster 0) reorders after it.
	if got[0].GID != 3 || got[0].Cluster != 2 {
		t.Errorf("first glyph = gid %d cl %d, want base KA (3, cl 2): %+v", got[0].GID, got[0].Cluster, got)
	}
	if got[1].GID != 4 || got[1].Cluster != 0 {
		t.Errorf("second glyph = gid %d cl %d, want repha (4, cl 0): %+v", got[1].GID, got[1].Cluster, got)
	}
}

// TestRealTaiThamSakot shapes Tai Tham RA + SAKOT + KA and asserts the sakot
// joins the two consonants into one cluster: no dotted circle is inserted and all
// glyphs share one syllable (so the count is exactly the three input signs).
func TestRealTaiThamSakot(t *testing.T) {
	f := testFont(t, "NotoSansTaiTham-Regular.ttf")
	got := Shape(f.NewFace(128), string([]rune{0x1A23, 0x1A60, 0x1A20}), Options{Script: "lana"})
	if len(got) != 3 {
		t.Fatalf("sakot cluster -> %d glyphs, want 3 (joined, no dotted circle): %+v", len(got), got)
	}
	// Segmentation must keep the three signs in a single cluster.
	cats := []useCat{useClass(0x1A23), useClass(0x1A60), useClass(0x1A20)}
	if segs := useSegment(cats); len(segs) != 1 || segs[0] != [2]int{0, 3} {
		t.Errorf("sakot did not join: segments = %v", segs)
	}
}
