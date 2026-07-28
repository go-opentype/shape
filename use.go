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
// Scope note: this is the general USE model. Per-script quirks that HarfBuzz
// special-cases (Sakot handling, split-vowel decomposition, pref/rphf
// feature-based reordering, dotted-circle insertion for defective clusters) are
// not reproduced; only property-based reordering (R, VPre, VMPre) is performed.

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
			i = scanIndependent(cats, i)
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
		if cats[j] == ucH && j+1 < n && (cats[j+1] == ucB || cats[j+1] == ucGB) {
			j += 2
			if j < n && cats[j] == ucVS {
				j++
			}
			j = scanMods(cats, j)
		} else if cats[j] == ucSUB {
			j++
			j = scanMods(cats, j)
		} else {
			break
		}
	}
	if j < n && (cats[j] == ucH || cats[j] == ucRK) {
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

// useReorder performs USE property-based reordering: within each cluster the
// pre-base vowel modifiers and pre-base vowels move ahead of the base and a
// leading repha moves after it. It permutes the glyph run and its clusters,
// using each glyph's source-rune category (cats) and cluster id (syll).
func useReorder(run []opentype.GlyphIndex, clusters []int, cats []useCat, syll []int) ([]opentype.GlyphIndex, []int) {
	n := len(run)
	if n == 0 {
		return run, clusters
	}
	gc := make([]useCat, n)
	gsy := make([]int, n)
	for j := 0; j < n; j++ {
		gc[j] = cats[clusters[j]]
		gsy[j] = syll[clusters[j]]
	}
	order := make([]int, 0, n)
	for a := 0; a < n; {
		b := a
		for b < n && gsy[b] == gsy[a] {
			b++
		}
		order = append(order, reorderRange(gc, a, b)...)
		a = b
	}
	nr := make([]opentype.GlyphIndex, n)
	nc := make([]int, n)
	for k, idx := range order {
		nr[k] = run[idx]
		nc[k] = clusters[idx]
	}
	return nr, nc
}

// reorderRange returns the indices of [a,b) in reordered order: pre-base vowel
// modifiers, then pre-base vowels, then the remaining glyphs (with a leading
// repha shifted past its base).
func reorderRange(gc []useCat, a, b int) []int {
	var vmpre, vpre, rest []int
	for i := a; i < b; i++ {
		switch gc[i] {
		case ucVMPre:
			vmpre = append(vmpre, i)
		case ucVPre:
			vpre = append(vpre, i)
		default:
			rest = append(rest, i)
		}
	}
	rest = rephaReorder(gc, rest)
	out := make([]int, 0, b-a)
	out = append(out, vmpre...)
	out = append(out, vpre...)
	return append(out, rest...)
}

// rephaReorder moves a leading repha (R) to just after the first following base
// in rest, mutating and returning rest. A rest that does not start with a repha,
// or has no base after it, is returned unchanged.
func rephaReorder(gc []useCat, rest []int) []int {
	if len(rest) == 0 || gc[rest[0]] != ucR {
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

// shapeUSE runs the USE substitution and reordering pipeline over a logical-order
// glyph run: classify each rune, apply the default/basic GSUB features, reorder
// (pre-base vowels/modifiers before the base, repha after it), then apply the
// presentation GSUB features. It is called from shapeGSUB with a non-nil GSUB and
// returns the substituted, reordered run and its clusters; the caller applies
// GPOS and emits the glyphs.
func shapeUSE(g *opentype.GSUB, runes []rune, run []opentype.GlyphIndex, clusters []int, user []string) ([]opentype.GlyphIndex, []int) {
	cats := make([]useCat, len(runes))
	for i, r := range runes {
		cats[i] = useClass(r)
	}
	syll := useSyllableIDs(cats)
	run, clusters = g.ApplyMaskedTracked(run, clusters, useStageI())
	run, clusters = useReorder(run, clusters, cats, syll)
	return g.ApplyMaskedTracked(run, clusters, useStageII(user))
}
