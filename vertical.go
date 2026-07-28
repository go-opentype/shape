// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"github.com/go-opentype/opentype"
)

// This file implements a vertical writing mode (CJK tategaki): glyphs laid out
// top to bottom in a column rather than left to right. It applies the OpenType
// vert/vrt2 GSUB features (which swap horizontal glyph forms for their upright
// vertical variants) and positions each glyph with the font's vertical metrics —
// the per-glyph vertical advance from vmtx (or one em when the font lacks it),
// the vhea line metrics, and the VORG vertical origin — so glyphs stack down a
// centred vertical baseline with a non-zero YAdvance each.
//
// Bidi reordering is not applied: a vertical column reads top to bottom in
// logical order.

// shapeVertical lays a run out top to bottom. Each output glyph carries its
// vertical advance in YAdvance, is centred horizontally on the vertical baseline
// (a negative XOffset of half its horizontal advance), and takes its YOffset
// from the vertical origin (VORG when present, otherwise the vhea ascender).
func shapeVertical(face *opentype.Face, runes []rune, opts Options) []Glyph {
	font := face.Font()
	scale := face.Scale()

	run := make([]opentype.GlyphIndex, len(runes))
	clusters := make([]int, len(runes))
	for i, r := range runes {
		gid, _ := font.GlyphIndex(r)
		run[i] = gid
		clusters[i] = i
	}

	// vert/vrt2 select the upright vertical glyph forms.
	if g := font.GSUB(); g != nil {
		feats := appendUser([]opentype.FeatureApp{{Tag: "vert"}, {Tag: "vrt2"}}, opts.Features)
		run, clusters = g.ApplyMaskedTracked(run, clusters, feats)
	}

	out := make([]Glyph, len(run))
	for i, gid := range run {
		r := runes[clusters[i]]
		hadv := px(font.GlyphAdvance(gid), scale)
		out[i] = Glyph{
			GID:      gid,
			Cluster:  clusters[i],
			YAdvance: face.VerticalAdvance(r),
			XOffset:  -hadv / 2,
			YOffset:  verticalYOffset(face, r),
			Scale:    1,
		}
	}
	return out
}

// verticalYOffset returns the y placement of r's glyph relative to the vertical
// pen: its VORG vertical origin when the font supplies one, otherwise the vhea
// ascender (zero when the font has neither).
func verticalYOffset(face *opentype.Face, r rune) int {
	if y, ok := face.VerticalOrigin(r); ok {
		return y
	}
	ascent, _, _ := face.VerticalMetrics()
	return ascent
}
