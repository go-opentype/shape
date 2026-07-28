// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"math"
	"strings"

	"github.com/go-opentype/bidi"
	"github.com/go-opentype/opentype"
)

// Glyph is one positioned glyph of a shaped run, in visual (left-to-right)
// order. GID is the glyph to draw; Cluster is the logical (rune) index in the
// source text the glyph derives from; the advances move the pen after drawing
// and the offsets place the glyph relative to the pen — all in whole pixels at
// the face's size.
type Glyph struct {
	GID      opentype.GlyphIndex
	Cluster  int
	XAdvance int
	YAdvance int
	XOffset  int
	YOffset  int
	// Scale is the factor to draw the glyph at relative to the face's size:
	// 1.0 for ordinary shaping, a fraction for the scaled-down signs of an
	// Egyptian Hieroglyph quadrat.
	Scale float64
}

// Options configures a Shape call. The zero value shapes with an automatic base
// direction (from the first strong character), a script auto-detected from the
// text, and no extra features.
type Options struct {
	// Direction is the base paragraph direction (bidi.LeftToRight,
	// bidi.RightToLeft or bidi.Auto). The zero value is bidi.LeftToRight.
	Direction bidi.Direction
	// Script forces the shaping script: "arab" for Arabic, "latn"/"dflt" (or
	// any other value) for the default shaper. Empty auto-detects: any
	// Arabic-block rune selects "arab", otherwise "dflt".
	Script string
	// Features lists extra OpenType feature tags to activate, applied over the
	// whole run in both the substitution and positioning stages (a tag with no
	// matching lookups is a no-op).
	Features []string
	// Vertical selects vertical writing mode (CJK tategaki): glyphs are laid
	// out top to bottom, the vert/vrt2 features select upright vertical forms,
	// and each glyph carries its vertical advance in YAdvance. Egyptian
	// Hieroglyph runs always use quadrat layout regardless of this flag.
	Vertical bool
}

// Shape turns text into a positioned glyph run in visual order. It resolves the
// bidirectional embedding levels, maps each rune to a glyph, applies GSUB
// (positionally for Arabic cursive joining, whole-run for ligatures and
// contextual alternates), positions the result with GPOS (kerning and mark
// attachment), and emits the glyphs left-to-right with per-glyph advances and
// offsets in pixels. An empty text, or a face whose font lacks GSUB/GPOS,
// simply skips the corresponding stage.
func Shape(face *opentype.Face, text string, opts Options) []Glyph {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	// Compose any conjoining Hangul jamo (L·V·T) into precomposed syllables
	// before the run is built, so they cmap + shape like ordinary Korean.
	runes = composeHangul(runes)
	if isEgyptianRun(opts.Script, runes) {
		return shapeEgyptian(face, runes, opts)
	}
	if opts.Vertical {
		return shapeVertical(face, runes, opts)
	}
	font := face.Font()
	levels := bidi.ResolveLevels(runes, opts.Direction)
	script := resolveScript(opts.Script, runes)
	forms := bidi.JoinForms(runes)

	// Logical-order glyph run, one glyph per rune; cluster = logical index.
	run := make([]opentype.GlyphIndex, len(runes))
	clusters := make([]int, len(runes))
	for i, r := range runes {
		gid, _ := font.GlyphIndex(r) // unmapped -> 0 (.notdef)
		run[i] = gid
		clusters[i] = i
	}

	// GSUB substitution, in logical order (positional for Arabic), tracking the
	// source cluster of every glyph so the joining masks can be rebuilt on the
	// decomposed run and the output can be mapped back to the text.
	if g := font.GSUB(); g != nil {
		run, clusters = shapeGSUB(font, g, run, clusters, runes, script, forms, opts.Features)
	}

	// Base advances (font units) for the substituted run.
	advances := make([]int, len(run))
	for i, gid := range run {
		advances[i] = font.GlyphAdvance(gid)
	}

	// GPOS positioning, in logical order.
	var positions []opentype.GlyphPosition
	if p := font.GPOS(); p != nil {
		positions = p.Position(run, advances, gposFeatures(script, opts.Features)...)
	}

	// Reorder to visual (left-to-right) order using each glyph's cluster level.
	glyphLevels := make([]bidi.Level, len(run))
	for i, c := range clusters {
		glyphLevels[i] = levels[c]
	}
	order := bidi.Reorder(nil, glyphLevels)

	scale := face.Scale()
	out := make([]Glyph, len(order))
	for k, idx := range order {
		g := Glyph{
			GID:      run[idx],
			Cluster:  clusters[idx],
			XAdvance: px(advances[idx], scale),
			Scale:    1,
		}
		if positions != nil {
			g.XAdvance = px(advances[idx]+positions[idx].XAdvance, scale)
			g.YAdvance = px(positions[idx].YAdvance, scale)
			g.XOffset = px(positions[idx].XOffset, scale)
			g.YOffset = px(positions[idx].YOffset, scale)
		}
		out[k] = g
	}
	return out
}

// shapeGSUB applies the GSUB substitution stage and returns the substituted run
// with its propagated clusters. Arabic is a two-step pass: ccmp first (which in
// real fonts decomposes each letter into a rasm skeleton plus dot marks), then
// the joining forms isol/init/medi/fina — rebuilt against the decomposed run
// from each glyph's source-rune joining form — followed by rlig/liga/calt. The
// default path applies ccmp/liga/clig over the whole run. User features follow,
// over the whole run. An Indic script routes to the syllable-based Indic shaper
// (shapeIndic), which needs the source runes for categorization and reordering.
func shapeGSUB(font *opentype.Font, g *opentype.GSUB, run []opentype.GlyphIndex, clusters []int, runes []rune, script string, forms []bidi.JoinForm, user []string) ([]opentype.GlyphIndex, []int) {
	if cfg, ok := indicConfigForTag(script); ok {
		return shapeIndic(font, g, runes, cfg, user)
	}
	if script == "use" {
		return shapeUSE(font, g, runes, user)
	}
	if script == "arab" {
		// ccmp first, tracked, so the joining masks align to the (possibly
		// decomposed) run rather than the original letters.
		run, clusters = g.ApplyMaskedTracked(run, clusters, []opentype.FeatureApp{{Tag: "ccmp"}})
		isol, init, medi, fina := formMasks(clusters, forms)
		feats := []opentype.FeatureApp{
			{Tag: "isol", Positions: isol},
			{Tag: "init", Positions: init},
			{Tag: "medi", Positions: medi},
			{Tag: "fina", Positions: fina},
			{Tag: "rlig"},
			{Tag: "liga"},
			{Tag: "calt"},
		}
		return g.ApplyMaskedTracked(run, clusters, appendUser(feats, user))
	}
	feats := []opentype.FeatureApp{{Tag: "ccmp"}, {Tag: "liga"}, {Tag: "clig"}}
	return g.ApplyMaskedTracked(run, clusters, appendUser(feats, user))
}

// resolveScript picks the shaping script: an explicit "arab" (case-insensitive)
// forces Arabic and an explicit Indic tag (old "deva" or v2 "dev2" form) forces
// that Indic script; an explicit USE script tag (thai, khmr, lana, ...) forces
// the Universal Shaping Engine; an empty value auto-detects Arabic, then Indic,
// then USE, from the runes; anything else (including "latn"/"dflt") selects the
// default shaper.
func resolveScript(want string, runes []rune) string {
	if strings.ToLower(want) == "arab" {
		return "arab"
	}
	if want == "" {
		if hasArabic(runes) {
			return "arab"
		}
		if cfg, ok := indicConfigForRunes(runes); ok {
			return cfg.tag
		}
		if hasUSE(runes) {
			return "use"
		}
		return "dflt"
	}
	if cfg, ok := indicConfigForTag(want); ok {
		return cfg.tag
	}
	if useScriptTags[strings.ToLower(want)] {
		return "use"
	}
	return "dflt"
}

// gposFeatures returns the GPOS feature tags for the run: kern/mark/mkmk/curs
// for Arabic, kern/dist/abvm/blwm/mark/mkmk for the Indic and USE paths,
// kern/mark for the default path, plus any user features.
func gposFeatures(script string, user []string) []string {
	var feats []string
	switch {
	case script == "arab":
		feats = []string{"kern", "mark", "mkmk", "curs"}
	case isIndicTag(script), script == "use":
		feats = []string{"kern", "dist", "abvm", "blwm", "mark", "mkmk"}
	default:
		feats = []string{"kern", "mark"}
	}
	return append(feats, user...)
}

// isIndicTag reports whether script is a canonical Indic script tag.
func isIndicTag(script string) bool {
	_, ok := indicConfigs[script]
	return ok
}

// appendUser appends each user feature tag as a whole-run FeatureApp.
func appendUser(feats []opentype.FeatureApp, user []string) []opentype.FeatureApp {
	for _, f := range user {
		feats = append(feats, opentype.FeatureApp{Tag: f})
	}
	return feats
}

// formMasks builds the isol/init/medi/fina position masks for a (possibly
// decomposed) glyph run: each glyph takes the joining form of its source rune,
// so a decomposed letter's rasm and its dot marks all carry that form. The
// isol/init/medi/fina GSUB lookups then substitute only the glyphs they cover
// (the rasm), leaving the marks untouched.
func formMasks(clusters []int, forms []bidi.JoinForm) (isol, init, medi, fina []bool) {
	n := len(clusters)
	isol = make([]bool, n)
	init = make([]bool, n)
	medi = make([]bool, n)
	fina = make([]bool, n)
	for j, c := range clusters {
		switch forms[c] {
		case bidi.Initial:
			init[j] = true
		case bidi.Medial:
			medi[j] = true
		case bidi.Final:
			fina[j] = true
		default: // bidi.Isolated
			isol[j] = true
		}
	}
	return isol, init, medi, fina
}

// hasArabic reports whether any rune is in an Arabic Unicode block.
func hasArabic(runes []rune) bool {
	for _, r := range runes {
		if isArabic(r) {
			return true
		}
	}
	return false
}

// isArabic reports whether r lies in an Arabic block: the main block, the
// Supplement, Extended-A, and Presentation Forms-A/-B.
func isArabic(r rune) bool {
	switch {
	case r >= 0x0600 && r <= 0x06FF, // Arabic
		r >= 0x0750 && r <= 0x077F, // Arabic Supplement
		r >= 0x08A0 && r <= 0x08FF, // Arabic Extended-A
		r >= 0xFB50 && r <= 0xFDFF, // Arabic Presentation Forms-A
		r >= 0xFE70 && r <= 0xFEFF: // Arabic Presentation Forms-B
		return true
	default:
		return false
	}
}

// px scales a font-unit value to whole pixels at scale, rounding to nearest.
func px(v int, scale float64) int { return int(math.Round(float64(v) * scale)) }
