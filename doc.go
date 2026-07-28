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
package shape
