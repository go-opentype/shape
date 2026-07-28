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
//   - Indic syllable reordering: Devanagari and the other Indic scripts are
//     split into syllables, each glyph is given a reorder position, the Indic
//     GSUB feature pipeline runs, and a stable sort moves pre-base (left) matras
//     before the base consonant and the reph to its script-specific slot — the
//     reordering a naive cmap-then-GSUB pass cannot express. See the Indic
//     section below.
//   - Egyptian Hieroglyph quadrats: the format-control characters
//     U+13430..U+1345F (vertical/horizontal joiners, corner insertions,
//     overlays, segment and enclosure delimiters, plus the Unicode 15 blank and
//     mirror additions) are parsed into a two-dimensional quadrat tree and laid
//     out geometrically, so a run of signs renders as compact blocks rather than
//     a flat row. See "Egyptian Hieroglyphs" below.
//   - Vertical writing mode (CJK tategaki): with Options.Vertical the vert/vrt2
//     features select upright vertical glyph forms and glyphs are stacked
//     top-to-bottom using the font's vertical metrics. See "Vertical" below.
//   - Universal Shaping Engine (USE): the general complex-script model for the
//     many scripts without a bespoke shaper (Thai, Lao, Khmer, Myanmar, Tibetan,
//     Javanese, Balinese, Buginese, Tai Tham, ...). Runs are classified into USE
//     syllabic categories, split into clusters, reordered (pre-base vowels and
//     modifiers before the base, repha after it), then run through the USE
//     GSUB/GPOS feature pipeline. See "USE" below.
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
// # Indic
//
// Devanagari, Bengali, Gurmukhi, Gujarati, Oriya, Tamil, Telugu, Kannada,
// Malayalam and Sinhala are shaped with the HarfBuzz "indic" model (the
// dev2/bng2/... v2 feature set; the old deva/beng/... tags are accepted too and
// normalized). Each run is split into syllables and shaped in HarfBuzz's two
// reordering passes:
//
//   - Two-part dependent vowels (the Bengali/Tamil/Malayalam/... O and AU signs)
//     are first decomposed into their canonical components, so each part reaches
//     its own reorder position.
//   - The base consonant is found with the script's base-position model
//     (BASE_POS_LAST for every script but Sinhala, which uses
//     BASE_POS_LAST_SINHALA), any reph is found in the script's encoding mode
//     (the implicit Ra + halant, Telugu's explicit Ra + halant + ZWJ, or
//     Malayalam's logical-order U+0D4E repha), and a defective cluster — one
//     opening with a dependent mark and no base — has a dotted circle (U+25CC)
//     inserted to carry the marks.
//   - Initial reordering: a stable sort by ladder position moves pre-base matras
//     before the base and parks the reph Ra at the front.
//   - The basic GSUB features run in order (locl, nukt, akhn, rphf masked to the
//     reph Ra, rkrf, pref masked to the pre-base-reordering Ra, blwf, half,
//     pstf, vatu, cjct), and whether rphf and pref actually fired is observed by
//     re-shaping the masked run.
//   - Final reordering: a fired reph moves to its per-script slot (otherwise the
//     Ra stays a pre-base consonant) and a fired pre-base-reordering Ra moves
//     ahead of the base.
//   - The presentation features run (init, pres, abvs, blws, psts, haln).
//
// GPOS then applies kern, dist, abvm, blwm, mark and mkmk. Character categories
// come from a table generated from the Unicode Character Database
// (Indic_Syllabic_Category and Indic_Positional_Category) by cmd/genindic.
//
// Indic edges deliberately left simple:
//
//   - Only the old (deva) and v2 (dev2) OpenType tags are recognized; a font
//     that files its features solely under a real script still resolves, since
//     GSUB/GPOS use the script-agnostic default fallback.
//
// # Scope
//
// Arabic, Indic, the Universal Shaping Engine (USE) and Latin/default (Latin,
// Cyrillic, Greek, CJK, ...) shaping are implemented, plus Egyptian Hieroglyph
// quadrat layout and vertical writing mode. USE covers the complex scripts that
// lack a bespoke shaper (Thai, Lao, Khmer, Myanmar, Tibetan, Javanese, ...),
// including the per-script behaviours sakot handling, split-vowel decomposition,
// pref/rphf feature-based reordering and dotted-circle insertion (see "USE"
// below). Cluster indices are exact for one-to-one
// substitutions (the Arabic positional forms) and best-effort, monotonic, when a
// substitution changes the run length (ligatures, decomposition, Indic and USE
// reordering).
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
//
// # USE
//
// The Universal Shaping Engine handles the complex scripts Unicode supports
// without a dedicated shaper — Thai, Lao, Khmer, Myanmar, Tibetan, Javanese,
// Balinese, Buginese, Tai Tham and many more. A run is forced to USE with an
// explicit USE script tag (thai, khmr, lana, ...) or auto-detected from its
// runes. Each rune is assigned a USE syllabic category (base, halant, pre/above/
// below/post vowel, vowel modifier, repha, ...), derived from the Unicode
// Indic_Syllabic_Category, Indic_Positional_Category and General_Category by
// cmd/genuse into the generated use_table.go; DeriveUSECategory is the shared
// derivation. The run is split into clusters by the USE syllable grammar
// (standard, virama-terminated, number-joiner, symbol and independent clusters),
// the default/basic GSUB features run (locl, ccmp, nukt, akhn, rphf, pref, rkrf,
// abvf, blwf, half, pstf, vatu, cjct), the run is reordered, the presentation
// GSUB features run (isol, init, medi, fina, abvs, blws, haln, pres, psts) and
// GPOS applies kern, dist, abvm, blwm, mark and mkmk.
//
// Reordering is both property- and feature-based: pre-base vowels (VPre) and
// pre-base vowel modifiers (VMPre) move ahead of the base, a consonant the pref
// feature turns into a pre-base form moves just before it, and a leading repha —
// a static Consonant_Preceding_Repha (R) or a cluster head the rphf feature
// ligates into a repha — moves after it. The other per-script behaviours are
// reproduced too: a sakot (the Tai Tham U+1A60, and the general Sakot class) or
// a halant joins a consonant to the following one across an optional ZWJ/ZWNJ so
// they stay one cluster; two-part dependent vowels (Tibetan, Balinese and the
// Chakma pair) are decomposed into their components before shaping so each part
// is classified and positioned on its own; and a defective cluster of bare
// combining marks receives an inserted U+25CC dotted-circle base. The ten
// dedicated-shaper Indic scripts (Devanagari, Bengali, Gurmukhi, Gujarati,
// Oriya, Tamil, Telugu, Kannada, Malayalam and Sinhala) are routed to the Indic
// shaper rather than USE.
package shape
