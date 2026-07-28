// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"math"
	"strings"

	"github.com/go-opentype/opentype"
)

// This file implements Egyptian Hieroglyph "quadrat" layout: the two-dimensional
// grouping of signs a run of hieroglyphs plus the Unicode format-control
// characters U+13430..U+1345F describes. A quadrat is a compact block in which
// signs are stacked (vertical joiner), set side by side (horizontal joiner),
// overlaid, inserted into a corner of another sign, or wrapped in an enclosure.
// The controls are an infix notation; this file parses a run into a quadrat
// forest and lays each quadrat out geometrically inside a single em box,
// computing a per-sign scale and X/Y offset so the block renders 2D rather than
// as a linear row.
//
// Font-specific EGYP quadrat features are not standardised in OpenType (the
// layout is a shaper responsibility), so the geometric layout below is always
// used. The font's GSUB ccmp feature is still consulted to pick font-preferred
// sign forms, position for position, before layout (see applyFontSubst); its
// GPOS is not applied, the geometric placement supersedes pair positioning.

// Egyptian Hieroglyph format-control code points (Unicode 12 base plus the
// Unicode 15 additions at U+13439..U+13440).
const (
	egVJoin     = 0x13430 // VERTICAL JOINER
	egHJoin     = 0x13431 // HORIZONTAL JOINER
	egInsertTS  = 0x13432 // INSERT AT TOP START
	egInsertBS  = 0x13433 // INSERT AT BOTTOM START
	egInsertTE  = 0x13434 // INSERT AT TOP END
	egInsertBE  = 0x13435 // INSERT AT BOTTOM END
	egOverlay   = 0x13436 // OVERLAY MIDDLE
	egBeginSeg  = 0x13437 // BEGIN SEGMENT
	egEndSeg    = 0x13438 // END SEGMENT
	egInsertM   = 0x13439 // INSERT AT MIDDLE (Unicode 15)
	egInsertT   = 0x1343A // INSERT AT TOP (Unicode 15)
	egInsertB   = 0x1343B // INSERT AT BOTTOM (Unicode 15)
	egBeginEnc  = 0x1343C // BEGIN ENCLOSURE (Unicode 15)
	egEndEnc    = 0x1343D // END ENCLOSURE (Unicode 15)
	egBeginWEnc = 0x1343E // BEGIN WALLED ENCLOSURE (Unicode 15)
	egEndWEnc   = 0x1343F // END WALLED ENCLOSURE (Unicode 15)
	egMirror    = 0x13440 // MIRROR HORIZONTALLY (Unicode 15)
	egBlankLo   = 0x13441 // FULL BLANK and following blank/shading additions
	egCtrlHi    = 0x1345F // last code point in the controls range
)

// egDefaultAdvance is the fallback quadrat side, in font units, used when a
// quadrat carries no drawable sign to measure (for example an all-blank block).
const egDefaultAdvance = 1000

// tkKind is the category of one token in an Egyptian run.
type tkKind int

const (
	tkSign       tkKind = iota // a drawable hieroglyph
	tkBlank                    // a blank/shading sign (occupies space, draws nothing)
	tkVJoin                    // vertical joiner
	tkHJoin                    // horizontal joiner
	tkOverlay                  // overlay-middle joiner
	tkInsert                   // an insertion control (corner placement)
	tkMirror                   // mirror-horizontally modifier
	tkBeginGroup               // begin segment / enclosure
	tkEndGroup                 // end segment / enclosure
)

// insKind names the region an INSERT control targets within its host sign.
type insKind int

const (
	insTopStart insKind = iota
	insBotStart
	insTopEnd
	insBotEnd
	insMiddle
	insTop
	insBottom
)

// token is one classified element of an Egyptian run.
type token struct {
	kind tkKind
	ins  insKind             // meaningful when kind == tkInsert
	enc  bool                // enclosure (vs. plain segment) when begin/end group
	gid  opentype.GlyphIndex // glyph, when kind == tkSign
	idx  int                 // source rune index (cluster)
}

// isEgyptianControl reports whether r is an Egyptian Hieroglyph format control.
func isEgyptianControl(r rune) bool { return r >= egVJoin && r <= egCtrlHi }

// isEgyptianSign reports whether r is a drawable Egyptian Hieroglyph: the base
// block (U+13000..U+1342F) or the Extended-A/B ranges (U+13460..U+143FF).
func isEgyptianSign(r rune) bool {
	return (r >= 0x13000 && r <= 0x1342F) || (r >= 0x13460 && r <= 0x143FF)
}

// isEgyptian reports whether r is any Egyptian Hieroglyph sign or control.
func isEgyptian(r rune) bool { return isEgyptianSign(r) || isEgyptianControl(r) }

// isEgyptianRun reports whether the run should be shaped as Egyptian: an
// explicit "egyp" script (case-insensitive) or any Egyptian rune present.
func isEgyptianRun(script string, runes []rune) bool {
	if strings.EqualFold(script, "egyp") {
		return true
	}
	for _, r := range runes {
		if isEgyptian(r) {
			return true
		}
	}
	return false
}

// classifyEgyptian maps a rune to its token kind (and, for inserts, the target
// region and, for group delimiters, whether it opens/closes an enclosure). A
// blank/shading addition becomes tkBlank; anything that is not a control is a
// sign (its glyph is resolved by the caller).
func classifyEgyptian(r rune) token {
	switch r {
	case egVJoin:
		return token{kind: tkVJoin}
	case egHJoin:
		return token{kind: tkHJoin}
	case egOverlay:
		return token{kind: tkOverlay}
	case egBeginSeg:
		return token{kind: tkBeginGroup}
	case egEndSeg:
		return token{kind: tkEndGroup}
	case egBeginEnc, egBeginWEnc:
		return token{kind: tkBeginGroup, enc: true}
	case egEndEnc, egEndWEnc:
		return token{kind: tkEndGroup, enc: true}
	case egMirror:
		return token{kind: tkMirror}
	case egInsertTS:
		return token{kind: tkInsert, ins: insTopStart}
	case egInsertBS:
		return token{kind: tkInsert, ins: insBotStart}
	case egInsertTE:
		return token{kind: tkInsert, ins: insTopEnd}
	case egInsertBE:
		return token{kind: tkInsert, ins: insBotEnd}
	case egInsertM:
		return token{kind: tkInsert, ins: insMiddle}
	case egInsertT:
		return token{kind: tkInsert, ins: insTop}
	case egInsertB:
		return token{kind: tkInsert, ins: insBottom}
	}
	if r >= egBlankLo && r <= egCtrlHi {
		return token{kind: tkBlank}
	}
	return token{kind: tkSign}
}

// nodeKind is the type of a node in a parsed quadrat tree.
type nodeKind int

const (
	nLeaf    nodeKind = iota // a single sign or blank
	nHoriz                   // signs set side by side
	nVert                    // signs stacked top to bottom
	nOverlay                 // signs overlaid in place
	nInsert                  // a sign inserted into a corner of a host
	nGroup                   // an explicit segment / enclosure wrapping one child
)

// node is one node of a quadrat tree.
type node struct {
	kind    nodeKind
	gid     opentype.GlyphIndex // leaf glyph
	cluster int                 // leaf source rune index
	blank   bool                // leaf is a blank
	kids    []*node             // horiz/vert children, or [host, inserted] for insert, or [child] for group
	ins     insKind             // insertion region, when kind == nInsert
	enc     bool                // enclosure (vs. plain segment), when kind == nGroup
}

// egCursor walks a token slice during recursive-descent parsing.
type egCursor struct {
	toks []token
	pos  int
}

func (c *egCursor) more() bool { return c.pos < len(c.toks) }

func (c *egCursor) peek() token { return c.toks[c.pos] }

// parseEgyptian tokenises runes and parses them into a quadrat forest: each
// element is one top-level quadrat, in reading order.
func parseEgyptian(runes []rune, font *opentype.Font) []*node {
	toks := make([]token, len(runes))
	for i, r := range runes {
		t := classifyEgyptian(r)
		t.idx = i
		if t.kind == tkSign {
			t.gid, _ = font.GlyphIndex(r)
		}
		toks[i] = t
	}
	c := &egCursor{toks: toks}
	var forest []*node
	for c.more() {
		forest = append(forest, c.parseVExpr())
	}
	return forest
}

// parseVExpr parses a vertical sequence: horizontal groups joined top to bottom
// by vertical joiners (the lowest-precedence combinator).
func (c *egCursor) parseVExpr() *node {
	kids := []*node{c.parseHExpr()}
	for c.more() && c.peek().kind == tkVJoin {
		c.pos++
		kids = append(kids, c.parseHExpr())
	}
	if len(kids) == 1 {
		return kids[0]
	}
	return &node{kind: nVert, kids: kids}
}

// parseHExpr parses a horizontal sequence: units joined side by side by
// horizontal joiners.
func (c *egCursor) parseHExpr() *node {
	kids := []*node{c.parseUnit()}
	for c.more() && c.peek().kind == tkHJoin {
		c.pos++
		kids = append(kids, c.parseUnit())
	}
	if len(kids) == 1 {
		return kids[0]
	}
	return &node{kind: nHoriz, kids: kids}
}

// parseUnit parses a primary optionally followed by postfix modifiers: corner
// insertions, overlays, and the mirror control (recognised as a geometric
// no-op).
func (c *egCursor) parseUnit() *node {
	base := c.parsePrimary()
	for c.more() {
		switch c.peek().kind {
		case tkInsert:
			ins := c.peek().ins
			c.pos++
			base = &node{kind: nInsert, ins: ins, kids: []*node{base, c.parsePrimary()}}
		case tkOverlay:
			c.pos++
			base = &node{kind: nOverlay, kids: []*node{base, c.parsePrimary()}}
		case tkMirror:
			c.pos++ // recognised; the geometric fallback does not mirror outlines
		default:
			return base
		}
	}
	return base
}

// parsePrimary parses one atom: a segment/enclosure group, a sign, or a blank.
// An unexpected control at atom position (a stray joiner or end-group) is
// consumed and treated as a blank so parsing always makes progress.
func (c *egCursor) parsePrimary() *node {
	if !c.more() {
		// A control (a joiner or a begin-group) was the last token, leaving no
		// operand; supply an empty blank so parsing terminates.
		return &node{kind: nLeaf, blank: true}
	}
	t := c.peek()
	switch t.kind {
	case tkBeginGroup:
		enc := t.enc
		c.pos++
		child := c.parseVExpr()
		if c.more() && c.peek().kind == tkEndGroup {
			c.pos++
		}
		return &node{kind: nGroup, enc: enc, kids: []*node{child}}
	case tkSign:
		c.pos++
		return &node{kind: nLeaf, gid: t.gid, cluster: t.idx}
	case tkBlank:
		c.pos++
		return &node{kind: nLeaf, blank: true, cluster: t.idx}
	default:
		c.pos++
		return &node{kind: nLeaf, blank: true, cluster: t.idx}
	}
}

// leafSigns returns the non-blank leaves of a quadrat, in reading order.
func leafSigns(n *node, out []*node) []*node {
	if n.kind == nLeaf {
		if !n.blank {
			out = append(out, n)
		}
		return out
	}
	for _, k := range n.kids {
		out = leafSigns(k, out)
	}
	return out
}

// applyFontSubst replaces each sign glyph with the font's ccmp-preferred form,
// position for position. A substitution that changes the glyph count (a
// ligature) is ignored so the quadrat structure the controls describe is
// preserved. It writes the chosen glyphs back into the leaves.
func applyFontSubst(g *opentype.GSUB, leaves []*node, user []string) {
	if g == nil {
		return
	}
	if len(leaves) == 0 {
		return
	}
	gids := make([]opentype.GlyphIndex, len(leaves))
	for i, l := range leaves {
		gids[i] = l.gid
	}
	out := g.ApplyMasked(gids, appendUser([]opentype.FeatureApp{{Tag: "ccmp"}}, user))
	if len(out) != len(leaves) {
		return
	}
	for i, l := range leaves {
		l.gid = out[i]
	}
}

// placed is a leaf sign positioned in its quadrat's normalised [0,1]x[0,1] box,
// y measured downward from the top.
type placed struct {
	gid        opentype.GlyphIndex
	cluster    int
	x, y, w, h float64
}

// layoutNode assigns each non-blank leaf a normalised sub-rectangle of the
// (x,y,w,h) box, appending the results to out. Horizontal groups split the box
// into equal columns, vertical groups into equal rows; an overlay places both
// children over the whole box; an insertion keeps the host on the whole box and
// drops the inserted sign into the corner its control names; a group insets the
// box for an enclosure border.
func layoutNode(n *node, x, y, w, h float64, out []placed) []placed {
	switch n.kind {
	case nLeaf:
		if n.blank {
			return out
		}
		return append(out, placed{gid: n.gid, cluster: n.cluster, x: x, y: y, w: w, h: h})
	case nHoriz:
		cw := w / float64(len(n.kids))
		for i, k := range n.kids {
			out = layoutNode(k, x+float64(i)*cw, y, cw, h, out)
		}
		return out
	case nVert:
		ch := h / float64(len(n.kids))
		for i, k := range n.kids {
			out = layoutNode(k, x, y+float64(i)*ch, w, ch, out)
		}
		return out
	case nOverlay:
		for _, k := range n.kids {
			out = layoutNode(k, x, y, w, h, out)
		}
		return out
	case nInsert:
		out = layoutNode(n.kids[0], x, y, w, h, out)
		ix, iy, iw, ih := insertRect(n.ins, x, y, w, h)
		return layoutNode(n.kids[1], ix, iy, iw, ih, out)
	default: // nGroup
		if n.enc {
			const inset = 0.15
			return layoutNode(n.kids[0], x+inset*w, y+inset*h, w*(1-2*inset), h*(1-2*inset), out)
		}
		return layoutNode(n.kids[0], x, y, w, h, out)
	}
}

// insertRect returns the sub-rectangle an insertion occupies within its host's
// (x,y,w,h) box: a quarter-size corner, edge, or centred region per the control.
func insertRect(ins insKind, x, y, w, h float64) (float64, float64, float64, float64) {
	const s = 0.4 // inserted sign occupies a fraction of the host box
	sw, sh := w*s, h*s
	switch ins {
	case insTopStart:
		return x, y, sw, sh
	case insBotStart:
		return x, y + h - sh, sw, sh
	case insTopEnd:
		return x + w - sw, y, sw, sh
	case insBotEnd:
		return x + w - sw, y + h - sh, sw, sh
	case insMiddle:
		return x + (w-sw)/2, y + (h-sh)/2, sw, sh
	case insTop:
		return x + (w-sw)/2, y, sw, sh
	default: // insBottom
		return x + (w-sw)/2, y + h - sh, sw, sh
	}
}

// shapeEgyptian lays out an Egyptian Hieroglyph run as quadrats. Each top-level
// quadrat occupies one em-square block: its signs are scaled down and offset so
// the group renders as a compact 2D arrangement, and the block's advance is
// carried on its last glyph. Signs are emitted in reading order.
func shapeEgyptian(face *opentype.Face, runes []rune, opts Options) []Glyph {
	font := face.Font()
	scale := face.Scale()
	forest := parseEgyptian(runes, font)
	gsub := font.GSUB()

	var out []Glyph
	for _, quad := range forest {
		leaves := leafSigns(quad, nil)
		applyFontSubst(gsub, leaves, opts.Features)

		// Quadrat side: the widest sign advance in the block (a full sign cell).
		sideFU := egDefaultAdvance
		for _, l := range leaves {
			if a := font.GlyphAdvance(l.gid); a > sideFU {
				sideFU = a
			}
		}
		sideF := float64(sideFU) * scale
		advance := int(math.Round(sideF))

		positions := layoutNode(quad, 0, 0, 1, 1, nil)
		start := len(out)
		for _, p := range positions {
			sc := math.Min(p.w, p.h)
			naturalWF := float64(font.GlyphAdvance(p.gid)) * scale
			glyphWF := sc * naturalWF
			cellXF := p.x * sideF
			cellWF := p.w * sideF
			out = append(out, Glyph{
				GID:     p.gid,
				Cluster: p.cluster,
				XOffset: int(math.Round(cellXF + (cellWF-glyphWF)/2)),
				YOffset: int(math.Round((1 - (p.y + p.h)) * sideF)),
				Scale:   sc,
			})
		}
		// Carry the block advance on its last emitted glyph.
		if len(out) > start {
			out[len(out)-1].XAdvance = advance
		}
	}
	return out
}
