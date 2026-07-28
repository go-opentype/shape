// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

// This file implements the Universal Shaping Engine (USE): the general
// complex-script model HarfBuzz and DirectWrite use for the many Brahmic and
// other complex scripts that lack a bespoke shaper (Thai, Lao, Khmer, Myanmar,
// Tibetan, Javanese, Balinese, Buginese, Tai Tham, ...). It follows the OpenType
// USE specification's four moving parts:
//
//   - Character classification: each rune is assigned a USE syllabic category
//     (base, halant, pre/above/below/post vowel, vowel modifier, ...) derived
//     from the Unicode UISC/UIPC/UGC properties. The per-rune table lives in the
//     generated use_table.go (see cmd/genuse); DeriveUSECategory is the shared
//     derivation the generator drives.
//   - Cluster segmentation: the classified run is split into syllable clusters
//     via the USE syllable grammar (standard, virama-terminated, number-joiner,
//     symbol and independent clusters).
//   - Reordering: pre-base vowels (VPre) and pre-base vowel modifiers (VMPre)
//     move ahead of the base, and a leading repha (R) moves after it, in the
//     single late reordering phase the spec prescribes.
//   - Feature pipeline: the default/basic GSUB features run first, then the run
//     is reordered, then the presentation GSUB features and the GPOS positioning
//     features, reusing this package's GSUB/GPOS helpers.
//
// Beyond property-based reordering (R, VPre, VMPre) this implements the four
// per-script behaviours HarfBuzz special-cases: sakot handling (a consonant, a
// Tai Tham sakot or halant, and a following consonant stay one cluster, across
// an optional ZWJ/ZWNJ), split-vowel decomposition (two-part dependent vowels
// are split into components before shaping so each part is classified and
// reordered on its own), feature-based reordering (a consonant the pref feature
// substitutes reorders before the base and a cluster head the rphf feature
// ligates into a repha reorders after it) and dotted-circle insertion for
// defective clusters (a cluster of bare combining marks gets a U+25CC base).

import "github.com/go-opentype/opentype"

// useCat is a USE syllabic category. The zero value ucO ("Other") is what an
// unclassified rune maps to. The categories mirror the sigla of the OpenType USE
// specification; positional variants (Abv/Blw/Pre/Pst) are distinct categories
// because reordering and the syllable grammar key off them.
type useCat uint8

const (
	ucO     useCat = iota // Other
	ucB                   // Base
	ucN                   // Base (number)
	ucGB                  // Base (generic / placeholder)
	ucIND                 // Base (independent, standalone)
	ucCGJ                 // Combining grapheme joiner
	ucCM                  // Consonant modifier (no position)
	ucCMAbv               // Consonant modifier, above
	ucCMBlw               // Consonant modifier, below
	ucCS                  // Consonant with stacker
	ucF                   // Consonant final (no position)
	ucFAbv                // Consonant final, above
	ucFBlw                // Consonant final, below
	ucFPst                // Consonant final, post
	ucFM                  // Final modifier
	ucH                   // Halant / virama / invisible stacker
	ucHN                  // Halant (number joiner)
	ucM                   // Consonant medial (no position)
	ucMPre                // Consonant medial, pre
	ucMAbv                // Consonant medial, above
	ucMBlw                // Consonant medial, below
	ucMPst                // Consonant medial, post
	ucR                   // Repha
	ucRK                  // Reordering killer
	ucS                   // Symbol
	ucSUB                 // Consonant subjoined
	ucSk                  // Sakot
	ucV                   // Vowel (no position)
	ucVPre                // Vowel, pre-base
	ucVAbv                // Vowel, above
	ucVBlw                // Vowel, below
	ucVPst                // Vowel, post
	ucVM                  // Vowel modifier (no position)
	ucVMPre               // Vowel modifier, pre-base
	ucVMAbv               // Vowel modifier, above
	ucVMBlw               // Vowel modifier, below
	ucVMPst               // Vowel modifier, post
	ucVS                  // Variation selector
	ucWJ                  // Word joiner
	ucZWJ                 // Zero-width joiner
	ucZWNJ                // Zero-width non-joiner
)

// useScriptTags is the set of OpenType script tags routed to the USE shaper when
// forced via Options.Script. It is the set of Universal Shaping Engine scripts
// (excluding the dedicated-shaper scripts Arabic and the nine Indic scripts
// handled elsewhere). Auto-detection uses the generated useScriptRanges instead.
var useScriptTags = map[string]bool{
	"thai": true, "lao": true, "khmr": true, "mymr": true, "mym2": true,
	"tibt": true, "java": true, "bali": true, "sund": true, "bugi": true,
	"batk": true, "rjng": true, "cham": true, "lepc": true, "limb": true,
	"sylo": true, "saur": true, "mtei": true, "cakm": true, "shrd": true,
	"takr": true, "khoj": true, "sind": true, "mult": true, "tirh": true,
	"sidd": true, "modi": true, "newa": true, "gran": true, "bhks": true,
	"marc": true, "gonm": true, "gong": true, "soyo": true, "zanb": true,
	"dogr": true, "nand": true, "tglg": true, "hano": true, "buhd": true,
	"tagb": true, "phag": true, "adlm": true, "rohg": true, "mong": true,
	"sinh": true, "tale": true, "talu": true, "tavt": true, "lana": true,
	"ahom": true, "brah": true, "kthi": true, "khar": true, "mahj": true,
	"maka": true, "medf": true, "plrd": true, "hmng": true, "hmnp": true,
	"wcho": true, "yezi": true, "chrs": true, "diak": true, "dupl": true,
	"elym": true, "gong2": true, "kali": true, "mand": true, "mani": true,
	"nko": true, "ougr": true, "phlp": true, "sogd": true, "sogo": true,
	"tfng": true, "tnsa": true, "toto": true, "vith": true, "cpmn": true,
	"kits": true, "gara": true, "gukh": true, "krai": true, "onao": true,
	"sunu": true, "todr": true, "tutg": true, "nagm": true, "kawi": true,
}

// useRange assigns a USE category to an inclusive code point range [lo,hi]. The
// generated useRanges slice is sorted by lo and non-overlapping.
type useRange struct {
	lo, hi uint32
	cat    useCat
}

// u32range is an inclusive code point range used for USE script detection.
type u32range struct{ lo, hi uint32 }

// useClass returns the USE category of a rune, ucO when it is not in the
// generated table (the common case for non-complex scripts).
func useClass(r rune) useCat {
	u := uint32(r)
	lo, hi := 0, len(useRanges)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		e := useRanges[mid]
		switch {
		case u < e.lo:
			hi = mid
		case u > e.hi:
			lo = mid + 1
		default:
			return e.cat
		}
	}
	return ucO
}

// isUSERune reports whether a rune belongs to a script the USE model handles.
func isUSERune(r rune) bool {
	u := uint32(r)
	lo, hi := 0, len(useScriptRanges)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		e := useScriptRanges[mid]
		switch {
		case u < e.lo:
			hi = mid
		case u > e.hi:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}

// hasUSE reports whether any rune is from a USE-handled script.
func hasUSE(runes []rune) bool {
	for _, r := range runes {
		if isUSERune(r) {
			return true
		}
	}
	return false
}

// DeriveUSECategory maps a rune's Unicode Indic_Syllabic_Category (uisc),
// Indic_Positional_Category (uipc) and General_Category (ugc) to its USE
// category, returned as the specification sigla (for example "B", "VAbv" or
// "VMPst"), or "O" for characters USE treats as Other. Positional families are
// refined from uipc. It implements the derivation table of the OpenType USE
// specification (minus the Arabic-joining and hieroglyph clauses, which are
// handled by dedicated shapers) and is exported so cmd/genuse can build the
// committed classification table without duplicating the mapping.
func DeriveUSECategory(uisc, uipc, ugc string) string {
	lo := ugc == "Lo"
	switch uisc {
	case "Number", "Consonant", "Consonant_Head_Letter", "Tone_Letter", "Vowel_Independent":
		return "B"
	case "Avagraha", "Bindu":
		if lo {
			return "B"
		}
	case "Consonant_Final":
		if lo {
			return "B"
		}
	case "Consonant_Medial":
		if lo {
			return "B"
		}
	case "Consonant_Subjoined":
		if lo {
			return "B"
		}
	case "Vowel":
		if lo {
			return "B"
		}
	case "Vowel_Dependent":
		if lo {
			return "B"
		}
	}
	switch uisc {
	case "Nukta", "Gemination_Mark", "Consonant_Killer", "Symbol_Modifier":
		return "CM" + useModPos(uipc)
	case "Consonant_With_Stacker":
		return "CS"
	case "Consonant_Final", "Consonant_Succeeding_Repha":
		return "F" + useFinalPos(uipc)
	case "Syllable_Modifier":
		return "FM"
	case "Consonant_Placeholder":
		return "GB"
	case "Virama", "Invisible_Stacker":
		return "H"
	case "Number_Joiner":
		return "HN"
	case "Consonant_Dead", "Modifying_Letter", "Other":
		return "IND"
	case "Consonant_Medial":
		return "M" + useMedialPos(uipc)
	case "Brahmi_Joining_Number":
		return "N"
	case "Consonant_Preceding_Repha", "Consonant_Prefixed":
		return "R"
	case "Reordering_Killer":
		return "RK"
	case "Consonant_Subjoined":
		return "SUB"
	case "Vowel", "Vowel_Dependent":
		return "V" + useVowelPos(uipc)
	case "Pure_Killer":
		return "V"
	case "Bindu", "Tone_Mark", "Cantillation_Mark", "Register_Shifter", "Visarga":
		return "VM" + useVowelPos(uipc)
	case "Joiner":
		return "ZWJ"
	case "Non_Joiner":
		return "ZWNJ"
	}
	if ugc == "Po" {
		return "IND"
	}
	return "O"
}

// useVowelPos maps a UIPC value to the Pre/Abv/Blw/Pst suffix used for the V and
// VM families, or "" when the position does not resolve to one of them.
func useVowelPos(uipc string) string {
	switch uipc {
	case "Left", "Top_And_Left", "Top_And_Left_And_Right", "Left_And_Right", "Visual_Order_Left":
		return "Pre"
	case "Top", "Top_And_Right", "Top_And_Bottom", "Top_And_Bottom_And_Right":
		return "Abv"
	case "Bottom", "Overstruck", "Bottom_And_Right":
		return "Blw"
	case "Right":
		return "Pst"
	}
	return ""
}

// useMedialPos maps a UIPC value to the medial-consonant position suffix.
func useMedialPos(uipc string) string {
	switch uipc {
	case "Left":
		return "Pre"
	case "Top":
		return "Abv"
	case "Bottom":
		return "Blw"
	case "Right":
		return "Pst"
	}
	return ""
}

// useFinalPos maps a UIPC value to the final-consonant position suffix.
func useFinalPos(uipc string) string {
	switch uipc {
	case "Top":
		return "Abv"
	case "Bottom":
		return "Blw"
	case "Right":
		return "Pst"
	}
	return ""
}

// useModPos maps a UIPC value to the consonant-modifier position suffix.
func useModPos(uipc string) string {
	switch uipc {
	case "Top":
		return "Abv"
	case "Bottom":
		return "Blw"
	}
	return ""
}

// useStageI is the default/basic GSUB feature set applied before reordering:
// localized forms, composition, nukta, akhand, the rephf/pref reordering group
// and the orthographic-unit shaping group.
func useStageI() []opentype.FeatureApp {
	tags := []string{"locl", "ccmp", "nukt", "akhn", "rphf", "pref", "rkrf", "abvf", "blwf", "half", "pstf", "vatu", "cjct"}
	fa := make([]opentype.FeatureApp, len(tags))
	for i, t := range tags {
		fa[i] = opentype.FeatureApp{Tag: t}
	}
	return fa
}

// useStageII is the presentation GSUB feature set applied after reordering
// (topographical joining forms then standard typographic presentation), plus
// any user features.
func useStageII(user []string) []opentype.FeatureApp {
	tags := []string{"isol", "init", "medi", "fina", "abvs", "blws", "haln", "pres", "psts"}
	fa := make([]opentype.FeatureApp, len(tags))
	for i, t := range tags {
		fa[i] = opentype.FeatureApp{Tag: t}
	}
	return appendUser(fa, user)
}

// useSegment splits a classified run into syllable clusters, each returned as a
// [start,end) index pair, following the USE syllable grammar: standard and
// virama-terminated consonant clusters, number-joiner clusters, symbol clusters
// and single-code-point independent clusters. Trailing joiners (ZWJ/ZWNJ/CGJ)
// are absorbed into the preceding cluster.
func useSegment(cats []useCat) [][2]int {
	var out [][2]int
	i, n := 0, len(cats)
	for i < n {
		start := i
		switch cats[i] {
		case ucB, ucGB, ucR, ucCS:
			i = scanStandard(cats, i)
		case ucN:
			i = scanNumber(cats, i)
		case ucS:
			i = scanSymbol(cats, i)
		default:
			if isDefectiveLead(cats[i]) {
				i = scanBroken(cats, i)
			} else {
				i = scanIndependent(cats, i)
			}
		}
		for i < n && (cats[i] == ucZWJ || cats[i] == ucZWNJ || cats[i] == ucCGJ) {
			i++
		}
		out = append(out, [2]int{start, i})
	}
	return out
}

// scanStandard consumes a standard or virama-terminated cluster starting at i
// and returns the index just past it.
func scanStandard(cats []useCat, i int) int {
	n := len(cats)
	j := i
	if cats[j] == ucR || cats[j] == ucCS {
		j++
	}
	if j >= n || !(cats[j] == ucB || cats[j] == ucGB) {
		return j
	}
	j++
	if j < n && cats[j] == ucVS {
		j++
	}
	j = scanMods(cats, j)
	for j < n {
		if cats[j] == ucH || cats[j] == ucSk {
			// A halant or a Tai Tham sakot (Sk) joins this consonant to the
			// following one — across an optional ZWJ/ZWNJ that selects the
			// joining form — so the whole run stays a single cluster.
			k := j + 1
			if k < n && (cats[k] == ucZWJ || cats[k] == ucZWNJ) {
				k++
			}
			if k < n && (cats[k] == ucB || cats[k] == ucGB) {
				j = k + 1
				if j < n && cats[j] == ucVS {
					j++
				}
				j = scanMods(cats, j)
				continue
			}
		}
		if cats[j] == ucSUB {
			j++
			j = scanMods(cats, j)
			continue
		}
		break
	}
	if j < n && (cats[j] == ucH || cats[j] == ucSk || cats[j] == ucRK) {
		return j + 1
	}
	j = scanOpt(cats, j, ucMPre)
	j = scanOpt(cats, j, ucMAbv)
	j = scanOpt(cats, j, ucMBlw)
	j = scanOpt(cats, j, ucMPst)
	for _, c := range []useCat{ucVPre, ucVAbv, ucVBlw, ucVPst, ucVMPre, ucVMAbv, ucVMBlw, ucVMPst, ucFAbv, ucFBlw, ucFPst} {
		j = scanStar(cats, j, c)
	}
	return scanOpt(cats, j, ucFM)
}

// scanMods consumes zero or more above-modifiers then zero or more
// below-modifiers.
func scanMods(cats []useCat, j int) int {
	j = scanStar(cats, j, ucCMAbv)
	return scanStar(cats, j, ucCMBlw)
}

// scanStar consumes zero or more consecutive glyphs of category c.
func scanStar(cats []useCat, j int, c useCat) int {
	for j < len(cats) && cats[j] == c {
		j++
	}
	return j
}

// scanOpt consumes zero or one glyph of category c.
func scanOpt(cats []useCat, j int, c useCat) int {
	if j < len(cats) && cats[j] == c {
		return j + 1
	}
	return j
}

// scanNumber consumes a number-joiner / numeral cluster starting at i.
func scanNumber(cats []useCat, i int) int {
	n := len(cats)
	j := i + 1
	if j < n && cats[j] == ucVS {
		j++
	}
	for j+1 < n && cats[j] == ucHN && cats[j+1] == ucN {
		j += 2
		if j < n && cats[j] == ucVS {
			j++
		}
	}
	if j < n && cats[j] == ucHN {
		j++
	}
	return j
}

// scanSymbol consumes a symbol cluster starting at i.
func scanSymbol(cats []useCat, i int) int {
	n := len(cats)
	j := i + 1
	if j < n && cats[j] == ucVS {
		j++
	}
	return scanMods(cats, j)
}

// scanIndependent consumes a single-code-point independent cluster (an IND, O,
// reserved or WJ base) with an optional trailing variation selector.
func scanIndependent(cats []useCat, i int) int {
	j := i + 1
	if j < len(cats) && cats[j] == ucVS {
		j++
	}
	return j
}

// scanBroken consumes a defective (broken) cluster starting at i: a maximal run
// of combining marks with no base to hang on. Such a cluster is where a dotted
// circle is later inserted, so the marks have a base to attach to.
func scanBroken(cats []useCat, i int) int {
	n := len(cats)
	j := i
	for j < n && (isDefectiveLead(cats[j]) || cats[j] == ucVS) {
		j++
	}
	return j
}

// isDefectiveLead reports whether a category, appearing at the head of a
// cluster, makes that cluster defective: it is a dependent mark (halant, sakot,
// consonant/vowel modifier, medial, final, subjoined, vowel or vowel modifier)
// with no base of its own. A defective cluster receives an inserted dotted
// circle to act as its base.
func isDefectiveLead(c useCat) bool {
	switch c {
	case ucH, ucSk, ucHN, ucRK,
		ucCM, ucCMAbv, ucCMBlw,
		ucF, ucFAbv, ucFBlw, ucFPst, ucFM,
		ucM, ucMPre, ucMAbv, ucMBlw, ucMPst, ucSUB,
		ucV, ucVPre, ucVAbv, ucVBlw, ucVPst,
		ucVM, ucVMPre, ucVMAbv, ucVMBlw, ucVMPst:
		return true
	}
	return false
}

// useSyllableIDs assigns each rune index the id of the cluster it belongs to.
func useSyllableIDs(cats []useCat) []int {
	ids := make([]int, len(cats))
	for id, seg := range useSegment(cats) {
		for k := seg[0]; k < seg[1]; k++ {
			ids[k] = id
		}
	}
	return ids
}

// useReorder performs USE reordering. Within each cluster the pre-base vowel
// modifiers (VMPre) and pre-base vowels (VPre) move ahead of the base, any
// consonant the pref feature turned into a pre-base form (pref[i]) moves just
// before the base, and a leading repha — a static Consonant_Preceding_Repha (R)
// or a consonant the rphf feature ligated into a repha (rphf[i]) — moves after
// the base. It permutes the glyph run and its tracking positions, keyed by each
// glyph's source category (cats), cluster id (syll) and per-position feature
// flags, all indexed through pos (the tracked source position of glyph j).
func useReorder(run []opentype.GlyphIndex, pos []int, cats []useCat, syll []int, pref, rphf []bool) ([]opentype.GlyphIndex, []int) {
	n := len(run)
	if n == 0 {
		return run, pos
	}
	gc := make([]useCat, n)
	gsy := make([]int, n)
	gpref := make([]bool, n)
	grphf := make([]bool, n)
	for j := 0; j < n; j++ {
		p := pos[j]
		gc[j] = cats[p]
		gsy[j] = syll[p]
		gpref[j] = pref[p]
		grphf[j] = rphf[p]
	}
	order := make([]int, 0, n)
	for a := 0; a < n; {
		b := a
		for b < n && gsy[b] == gsy[a] {
			b++
		}
		order = append(order, reorderRange(gc, gpref, grphf, a, b)...)
		a = b
	}
	nr := make([]opentype.GlyphIndex, n)
	np := make([]int, n)
	for k, idx := range order {
		nr[k] = run[idx]
		np[k] = pos[idx]
	}
	return nr, np
}

// reorderRange returns the indices of [a,b) in reordered order: pre-base vowel
// modifiers, then pre-base vowels, then any pref-substituted (pre-base) glyphs,
// then the remaining glyphs (with a leading repha shifted past its base).
func reorderRange(gc []useCat, pref, rphf []bool, a, b int) []int {
	var vmpre, vpre, prefc, rest []int
	for i := a; i < b; i++ {
		switch {
		case gc[i] == ucVMPre:
			vmpre = append(vmpre, i)
		case gc[i] == ucVPre:
			vpre = append(vpre, i)
		case pref[i]:
			prefc = append(prefc, i)
		default:
			rest = append(rest, i)
		}
	}
	rest = rephaReorder(gc, rphf, rest)
	out := make([]int, 0, b-a)
	out = append(out, vmpre...)
	out = append(out, vpre...)
	out = append(out, prefc...)
	return append(out, rest...)
}

// rephaReorder moves a leading repha to just after the first following base in
// rest, mutating and returning rest. The head is a repha when it is a static
// Consonant_Preceding_Repha (R) or the rphf feature fired on it (rphf[rest[0]]).
// A rest that does not start with a repha, or has no base after it, is returned
// unchanged.
func rephaReorder(gc []useCat, rphf []bool, rest []int) []int {
	if len(rest) == 0 || !(gc[rest[0]] == ucR || rphf[rest[0]]) {
		return rest
	}
	bi := -1
	for k := 1; k < len(rest); k++ {
		if gc[rest[k]] == ucB || gc[rest[k]] == ucGB {
			bi = k
			break
		}
	}
	if bi < 0 {
		return rest
	}
	r := rest[0]
	copy(rest, rest[1:bi+1])
	rest[bi] = r
	return rest
}

// useSplitVowels maps a two-part USE dependent vowel to the sequence of
// single-part components it decomposes into (fully expanded, so no component is
// itself splittable). These are the canonical decompositions the USE model
// applies before shaping — the Tibetan and Balinese two-part vowel signs — plus
// the two Chakma vowels HarfBuzz decomposes by hand. Splitting lets each part be
// classified and reordered on its own, so a leading (pre-base) part moves ahead
// of the base while the trailing part stays after it. (Sinhala's two-part vowels
// are decomposed by the dedicated Indic shaper, which owns that script.)
var useSplitVowels = map[rune][]rune{
	0x0F73:  {0x0F71, 0x0F72},   // TIBETAN VOWEL SIGN II
	0x0F75:  {0x0F71, 0x0F74},   // TIBETAN VOWEL SIGN UU
	0x0F76:  {0x0FB2, 0x0F80},   // TIBETAN VOWEL SIGN VOCALIC R
	0x0F78:  {0x0FB3, 0x0F80},   // TIBETAN VOWEL SIGN VOCALIC L
	0x0F81:  {0x0F71, 0x0F80},   // TIBETAN VOWEL SIGN REVERSED II
	0x1B3B:  {0x1B3A, 0x1B35},   // BALINESE VOWEL SIGN RA REPA TEDUNG
	0x1B3D:  {0x1B3C, 0x1B35},   // BALINESE VOWEL SIGN LA LENGA TEDUNG
	0x1B40:  {0x1B3E, 0x1B35},   // BALINESE VOWEL SIGN TALING TEDUNG
	0x1B41:  {0x1B3F, 0x1B35},   // BALINESE VOWEL SIGN TALING REPA TEDUNG
	0x1B43:  {0x1B42, 0x1B35},   // BALINESE VOWEL SIGN PEPET TEDUNG
	0x1112E: {0x11131, 0x11127}, // CHAKMA VOWEL SIGN O
	0x1112F: {0x11132, 0x11127}, // CHAKMA VOWEL SIGN AU
}

// decomposeUSE expands the split vowels of runes into their components,
// returning the decomposed rune sequence and, per component, the index of the
// source rune it came from (both parts of a split vowel share the source rune's
// index).
func decomposeUSE(runes []rune) (dr []rune, src []int) {
	dr = make([]rune, 0, len(runes))
	src = make([]int, 0, len(runes))
	for i, r := range runes {
		if comp, ok := useSplitVowels[r]; ok {
			for _, c := range comp {
				dr = append(dr, c)
				src = append(src, i)
			}
			continue
		}
		dr = append(dr, r)
		src = append(src, i)
	}
	return dr, src
}

// insertDottedCircles inserts a U+25CC (dotted circle) at the head of every
// defective cluster — one whose leading category is a bare combining mark — so
// the marks have a base, matching HarfBuzz. It returns the augmented runes,
// their source indices and their categories; when no cluster is defective the
// inputs are returned unchanged.
func insertDottedCircles(dr []rune, src []int, cats []useCat) ([]rune, []int, []useCat) {
	insert := map[int]bool{}
	for _, seg := range useSegment(cats) {
		if isDefectiveLead(cats[seg[0]]) {
			insert[seg[0]] = true
		}
	}
	if len(insert) == 0 {
		return dr, src, cats
	}
	dcCat := useClass(dottedCircle)
	ndr := make([]rune, 0, len(dr)+len(insert))
	nsrc := make([]int, 0, len(dr)+len(insert))
	ncats := make([]useCat, 0, len(dr)+len(insert))
	for i := range dr {
		if insert[i] {
			ndr = append(ndr, dottedCircle)
			nsrc = append(nsrc, src[i])
			ncats = append(ncats, dcCat)
		}
		ndr = append(ndr, dr[i])
		nsrc = append(nsrc, src[i])
		ncats = append(ncats, cats[i])
	}
	return ndr, nsrc, ncats
}

// dottedCircle is U+25CC, the base a defective cluster's marks attach to.
const dottedCircle = 0x25CC

// detectUSEFeatures probes the pre-base (pref) and repha (rphf) GSUB features on
// each cluster of the cmap glyph run and returns, per glyph position, whether
// pref substituted that glyph (it becomes a pre-base form and reorders before
// the base) and whether rphf fired at the cluster head (it becomes a repha and
// reorders after the base).
func detectUSEFeatures(g *opentype.GSUB, run []opentype.GlyphIndex, cats []useCat) (pref, rphf []bool) {
	pref = make([]bool, len(run))
	rphf = make([]bool, len(run))
	for _, seg := range useSegment(cats) {
		s, e := seg[0], seg[1]
		sub := run[s:e]
		cl := make([]int, e-s)
		for i := range cl {
			cl[i] = s + i
		}
		// pref may fire at any consonant of the cluster; rphf only at its head.
		for c := range firedClusters(g, sub, cl, nil, "pref") {
			pref[c] = true
		}
		head := make([]bool, e-s)
		head[0] = true
		if firedClusters(g, sub, cl, head, "rphf")[s] {
			rphf[s] = true
		}
	}
	return pref, rphf
}

// firedClusters applies feature tag — restricted to mask, or over the whole
// cluster when mask is nil — to sub (a cluster's glyphs, whose source positions
// are cl) and returns the set of source positions whose glyph the feature
// substituted: a position whose glyph changed, or one a ligature consumed. It is
// how the pre-base (pref) and repha (rphf) reordering learn which glyphs to move.
func firedClusters(g *opentype.GSUB, sub []opentype.GlyphIndex, cl []int, mask []bool, tag string) map[int]bool {
	out, oc := g.ApplyMaskedTracked(append([]opentype.GlyphIndex(nil), sub...), append([]int(nil), cl...), []opentype.FeatureApp{{Tag: tag, Positions: mask}})
	in := make(map[int]opentype.GlyphIndex, len(cl))
	for i, c := range cl {
		in[c] = sub[i]
	}
	fired := map[int]bool{}
	present := map[int]bool{}
	for k, c := range oc {
		present[c] = true
		if in[c] != out[k] {
			fired[c] = true
		}
	}
	for _, c := range cl {
		if !present[c] {
			fired[c] = true
		}
	}
	return fired
}

// shapeUSE runs the full USE substitution and reordering pipeline over a
// logical-order run: split-vowel decomposition, classification, dotted-circle
// insertion for defective clusters, the default/basic GSUB features, feature-
// and property-based reordering (pre-base vowels/modifiers and pref consonants
// before the base, static or rphf repha after it), then the presentation GSUB
// features. It is called from shapeGSUB with a non-nil GSUB, maps runes to
// glyphs through font (decomposition and dotted circles introduce new runes),
// and returns the substituted, reordered run with clusters mapped back to the
// original source-rune indices; the caller applies GPOS and emits the glyphs.
func shapeUSE(font *opentype.Font, g *opentype.GSUB, runes []rune, user []string) ([]opentype.GlyphIndex, []int) {
	dr, src := decomposeUSE(runes)
	cats := make([]useCat, len(dr))
	for i, r := range dr {
		cats[i] = useClass(r)
	}
	dr, src, cats = insertDottedCircles(dr, src, cats)
	run := make([]opentype.GlyphIndex, len(dr))
	for i, r := range dr {
		run[i], _ = font.GlyphIndex(r)
	}
	syll := useSyllableIDs(cats)
	pref, rphf := detectUSEFeatures(g, run, cats)
	pos := make([]int, len(dr))
	for i := range pos {
		pos[i] = i
	}
	run, pos = g.ApplyMaskedTracked(run, pos, useStageI())
	run, pos = useReorder(run, pos, cats, syll, pref, rphf)
	run, pos = g.ApplyMaskedTracked(run, pos, useStageII(user))
	clusters := make([]int, len(pos))
	for i, p := range pos {
		clusters[i] = src[p]
	}
	return run, clusters
}
