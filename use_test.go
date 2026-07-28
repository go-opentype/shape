// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
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
	cases := []struct {
		gc   []useCat
		want []int
	}{
		{[]useCat{ucB, ucVAbv}, []int{0, 1}}, // no pre-base movement
		{[]useCat{ucB, ucVPre}, []int{1, 0}}, // VPre before base
		{[]useCat{ucVMPre, ucVPre, ucB}, []int{0, 1, 2}},
		{[]useCat{ucB, ucVPre, ucVMPre}, []int{2, 1, 0}}, // VMPre then VPre then base
	}
	for _, c := range cases {
		if got := reorderRange(c.gc, 0, len(c.gc)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("reorderRange(%v) = %v, want %v", c.gc, got, c.want)
		}
	}
}

func TestRephaReorder(t *testing.T) {
	cases := []struct {
		gc   []useCat
		rest []int
		want []int
	}{
		{[]useCat{ucR, ucB, ucVAbv}, []int{0, 1, 2}, []int{1, 0, 2}}, // R after base
		{[]useCat{ucR, ucGB}, []int{0, 1}, []int{1, 0}},              // base can be GB
		{[]useCat{ucR}, []int{0}, []int{0}},                          // R with no base
		{[]useCat{ucB}, []int{0}, []int{0}},                          // not a repha
		{nil, nil, nil},                                              // empty
	}
	for _, c := range cases {
		if got := rephaReorder(c.gc, c.rest); !reflect.DeepEqual(got, c.want) {
			t.Errorf("rephaReorder(%v,%v) = %v, want %v", c.gc, c.rest, got, c.want)
		}
	}
}

func TestUseReorder(t *testing.T) {
	// Empty run is returned unchanged.
	if r, c := useReorder(nil, nil, nil, nil); r != nil || c != nil {
		t.Errorf("useReorder(nil) = %v,%v want nil,nil", r, c)
	}
	// One syllable [B, VPre, VMPre] (runes 0,1,2) -> reordered VMPre,VPre,B.
	run := []opentype.GlyphIndex{10, 20, 30}
	clusters := []int{0, 1, 2}
	cats := []useCat{ucB, ucVPre, ucVMPre}
	syll := []int{0, 0, 0}
	gotRun, gotCl := useReorder(run, clusters, cats, syll)
	if want := []opentype.GlyphIndex{30, 20, 10}; !reflect.DeepEqual(gotRun, want) {
		t.Errorf("useReorder run = %v, want %v", gotRun, want)
	}
	if want := []int{2, 1, 0}; !reflect.DeepEqual(gotCl, want) {
		t.Errorf("useReorder clusters = %v, want %v", gotCl, want)
	}
	// Two syllables reorder independently: [B,VPre | B] -> [VPre,B | B].
	run2 := []opentype.GlyphIndex{10, 20, 30}
	cl2 := []int{0, 1, 2}
	cats2 := []useCat{ucB, ucVPre, ucB}
	syll2 := []int{0, 0, 1}
	gr, _ := useReorder(run2, cl2, cats2, syll2)
	if want := []opentype.GlyphIndex{20, 10, 30}; !reflect.DeepEqual(gr, want) {
		t.Errorf("two-syllable reorder = %v, want %v", gr, want)
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
