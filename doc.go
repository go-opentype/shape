// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

// Package shape is a HarfBuzz-lite complex-text shaper for the go-opentype
// stack. It turns a run of Unicode text into positioned glyphs in visual
// (left-to-right) order, ready to blit, applying the three things a naive
// cmap-then-GSUB pass gets wrong for real text:
//
//   - Bidirectional reordering (via github.com/go-opentype/bidi): resolve the
//     UAX #9 embedding levels and lay the glyphs out left-to-right, so a
//     right-to-left Arabic run is emitted in the order it is drawn.
//   - Arabic cursive joining: each letter's Unicode joining form (isolated,
//     initial, medial, final) is resolved, then the font's isol/init/medi/fina
//     GSUB features are applied positionally — each only at the glyphs in that
//     form — via opentype's ApplyMasked. Without this, Arabic renders as
//     disconnected isolated letters.
//   - Ligatures, mark attachment and kerning: GSUB ccmp/rlig/liga/calt then
//     GPOS kern/mark/mkmk/curs, so diacritics sit on their base and pairs kern.
//   - Egyptian Hieroglyph quadrats: the format-control characters
//     U+13430..U+1345F (vertical/horizontal joiners, corner insertions,
//     overlays, segment and enclosure delimiters, plus the Unicode 15 blank and
//     mirror additions) are parsed into a two-dimensional quadrat tree and laid
//     out geometrically, so a run of signs renders as compact blocks rather than
//     a flat row. See "Egyptian Hieroglyphs" below.
//   - Vertical writing mode (CJK tategaki): with Options.Vertical the vert/vrt2
//     features select upright vertical glyph forms and glyphs are stacked
//     top-to-bottom using the font's vertical metrics. See "Vertical" below.
//
// # Usage
//
//	face := font.NewFace(32)
//	glyphs := shape.Shape(face, "بيت", shape.Options{})
//	for _, g := range glyphs {
//		// g.GID is the glyph to draw; advance the pen by g.XAdvance,
//		// offset the glyph by (g.XOffset, g.YOffset). All in pixels.
//	}
//
// The base direction defaults to Auto (derived from the first strong
// character); the script is auto-detected from the text (any Arabic-block rune
// selects the Arabic shaper) unless Options.Script forces it.
//
// # Scope
//
// Arabic and Latin/default (Latin, Cyrillic, Greek, CJK, ...) shaping are
// implemented. Scripts that need glyph reordering or a state machine — Indic
// (Devanagari, ...), Thai/Lao, Khmer, Myanmar and the Universal Shaping Engine
// — are out of scope and shape as the default path (no reordering); they are
// future work. Cluster indices are exact for one-to-one substitutions (the
// Arabic positional forms) and best-effort, monotonic, when a substitution
// changes the run length (ligatures, decomposition).
//
// # Egyptian Hieroglyphs
//
// A run whose script is "egyp" (or that contains any Egyptian rune, U+13000 and
// above) is shaped as quadrats: the format controls U+13430..U+1345F are an
// infix notation grouping signs two-dimensionally. Vertical joiners stack signs,
// horizontal joiners set them side by side, overlays place one over another,
// insertions drop a sign into a corner or edge of a host, and segment/enclosure
// delimiters bracket sub-groups; the Unicode 15 blank/shading code points are
// treated as space-occupying blanks and the mirror control is recognised as a
// geometric no-op. Each top-level quadrat is laid out inside one em block: every
// sign gets a Scale below 1 and an X/Y offset placing it in the block, and the
// block's advance is carried on its last glyph.
//
// The quadrat layout is geometric: OpenType has no standard font-driven quadrat
// mechanism, so the control-character model above is implemented directly. The
// font's GSUB ccmp feature is consulted to pick font-preferred sign forms
// position-for-position before layout (a substitution that would change the sign
// count, such as a ligature, is suppressed to preserve the quadrat structure);
// font GPOS is not applied, the geometric placement supersedes it.
//
// # Vertical
//
// Options.Vertical selects vertical writing mode (CJK tategaki). The vert/vrt2
// GSUB features swap horizontal glyph forms for their upright vertical variants,
// and each glyph is positioned top-to-bottom: YAdvance is the glyph's vertical
// advance (from vmtx, or one em when the font has none), XOffset centres it on
// the vertical baseline, and YOffset is its vertical origin (VORG when present,
// otherwise the vhea ascender). Bidi reordering is not applied — a vertical
// column reads top-to-bottom in logical order.
package shape
