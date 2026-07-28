// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"testing"

	"github.com/go-opentype/fonts/notosansegyptianhieroglyphs"
	"github.com/go-opentype/opentype"
)

// egyFont is a layout-table-free font whose BMP letters A..D stand in for
// hieroglyph signs (classified as signs when the run is forced to script
// "egyp"), with distinct advances so quadrat geometry is observable.
func egyFont(t *testing.T) *opentype.Font {
	t.Helper()
	// 'A' is wider than egDefaultAdvance so a quadrat containing it sizes to the
	// sign rather than the fallback.
	return bareFont(t, map[rune]uint16{'A': 1, 'B': 2, 'C': 3, 'D': 4}, 1000,
		map[rune]int{'A': 1200, 'B': 400, 'C': 500, 'D': 800})
}

// ctrl returns an Egyptian format-control (or sign) code point as a string.
func ctrl(r rune) string { return string(r) }

func TestEgyClassifiers(t *testing.T) {
	if !isEgyptianControl(egVJoin) || isEgyptianControl(0x13000) {
		t.Error("isEgyptianControl misclassified")
	}
	for _, r := range []rune{0x13000, 0x1342F, 0x13460, 0x143FF} {
		if !isEgyptianSign(r) {
			t.Errorf("isEgyptianSign(U+%05X) = false", r)
		}
	}
	for _, r := range []rune{'A', 0x12FFF, 0x13440, 0x14400} {
		if isEgyptianSign(r) {
			t.Errorf("isEgyptianSign(U+%05X) = true", r)
		}
	}
	if !isEgyptian(0x13000) || !isEgyptian(egVJoin) || isEgyptian('A') {
		t.Error("isEgyptian misclassified")
	}
	if !isEgyptianRun("EGYP", []rune("abc")) {
		t.Error("script egyp not detected")
	}
	if !isEgyptianRun("", []rune{0x13000}) {
		t.Error("egyptian rune not detected")
	}
	if isEgyptianRun("", []rune("abc")) {
		t.Error("plain latin wrongly detected as egyptian")
	}
}

func TestClassifyEgyptian(t *testing.T) {
	cases := []struct {
		r    rune
		kind tkKind
		ins  insKind
		enc  bool
	}{
		{egVJoin, tkVJoin, 0, false},
		{egHJoin, tkHJoin, 0, false},
		{egOverlay, tkOverlay, 0, false},
		{egBeginSeg, tkBeginGroup, 0, false},
		{egEndSeg, tkEndGroup, 0, false},
		{egBeginEnc, tkBeginGroup, 0, true},
		{egEndEnc, tkEndGroup, 0, true},
		{egBeginWEnc, tkBeginGroup, 0, true},
		{egEndWEnc, tkEndGroup, 0, true},
		{egMirror, tkMirror, 0, false},
		{egInsertTS, tkInsert, insTopStart, false},
		{egInsertBS, tkInsert, insBotStart, false},
		{egInsertTE, tkInsert, insTopEnd, false},
		{egInsertBE, tkInsert, insBotEnd, false},
		{egInsertM, tkInsert, insMiddle, false},
		{egInsertT, tkInsert, insTop, false},
		{egInsertB, tkInsert, insBottom, false},
		{egBlankLo, tkBlank, 0, false},
		{egCtrlHi, tkBlank, 0, false},
		{0x13000, tkSign, 0, false},
	}
	for _, c := range cases {
		got := classifyEgyptian(c.r)
		if got.kind != c.kind || got.ins != c.ins || got.enc != c.enc {
			t.Errorf("classifyEgyptian(U+%05X) = %+v, want kind %d ins %d enc %v",
				c.r, got, c.kind, c.ins, c.enc)
		}
	}
}

// TestEgyVerticalJoin: a vertical joiner stacks two signs; both are scaled down
// and the second sits below the first.
func TestEgyVerticalJoin(t *testing.T) {
	f := egyFont(t)
	got := Shape(f.NewFace(100), "A"+ctrl(egVJoin)+"B", Options{Script: "egyp"})
	if len(got) != 2 {
		t.Fatalf("A|B -> %d glyphs, want 2: %+v", len(got), got)
	}
	if got[0].Scale != 0.5 || got[1].Scale != 0.5 {
		t.Errorf("scales = %v,%v want 0.5,0.5", got[0].Scale, got[1].Scale)
	}
	if !(got[0].YOffset > got[1].YOffset) {
		t.Errorf("second sign not below first: YOffsets %d,%d", got[0].YOffset, got[1].YOffset)
	}
	if got[0].Cluster != 0 || got[1].Cluster != 2 {
		t.Errorf("clusters = %d,%d want 0,2", got[0].Cluster, got[1].Cluster)
	}
}

// TestEgyHorizontalJoin: a horizontal joiner sets two signs side by side at the
// same height, the second to the right of the first.
func TestEgyHorizontalJoin(t *testing.T) {
	f := egyFont(t)
	got := Shape(f.NewFace(100), "A"+ctrl(egHJoin)+"B", Options{Script: "egyp"})
	if len(got) != 2 {
		t.Fatalf("A*B -> %d glyphs, want 2: %+v", len(got), got)
	}
	if got[0].YOffset != got[1].YOffset {
		t.Errorf("side-by-side signs differ in height: %d,%d", got[0].YOffset, got[1].YOffset)
	}
	if !(got[0].XOffset < got[1].XOffset) {
		t.Errorf("second sign not to the right: XOffsets %d,%d", got[0].XOffset, got[1].XOffset)
	}
	// The block advance is carried on the last glyph.
	if got[1].XAdvance == 0 || got[0].XAdvance != 0 {
		t.Errorf("advance not on last glyph: %+v", got)
	}
}

// TestEgyInsertions drives every insertion control: the host keeps full size,
// the inserted sign drops into a scaled corner/edge region.
func TestEgyInsertions(t *testing.T) {
	f := egyFont(t)
	for _, ins := range []rune{egInsertTS, egInsertBS, egInsertTE, egInsertBE, egInsertM, egInsertT, egInsertB} {
		got := Shape(f.NewFace(100), "A"+ctrl(ins)+"B", Options{Script: "egyp"})
		if len(got) != 2 {
			t.Fatalf("insert U+%05X -> %d glyphs: %+v", ins, len(got), got)
		}
		var host, inserted bool
		for _, g := range got {
			if g.Scale == 1 {
				host = true
			}
			if g.Scale == 0.4 {
				inserted = true
			}
		}
		if !host || !inserted {
			t.Errorf("insert U+%05X: host=%v inserted=%v %+v", ins, host, inserted, got)
		}
	}
}

// TestEgyOverlay overlays two signs in place: both keep full size and the same
// origin.
func TestEgyOverlay(t *testing.T) {
	f := egyFont(t)
	got := Shape(f.NewFace(100), "A"+ctrl(egOverlay)+"B", Options{Script: "egyp"})
	if len(got) != 2 {
		t.Fatalf("overlay -> %d glyphs: %+v", len(got), got)
	}
	if got[0].Scale != 1 || got[1].Scale != 1 {
		t.Errorf("overlay scales = %v,%v want 1,1", got[0].Scale, got[1].Scale)
	}
}

// TestEgyMirror: the mirror control is recognised and consumed, leaving the sign
// count unchanged.
func TestEgyMirror(t *testing.T) {
	f := egyFont(t)
	got := Shape(f.NewFace(100), "A"+ctrl(egMirror), Options{Script: "egyp"})
	if len(got) != 1 {
		t.Fatalf("A+mirror -> %d glyphs, want 1: %+v", len(got), got)
	}
}

// TestEgySegment groups a horizontal pair inside a plain segment.
func TestEgySegment(t *testing.T) {
	f := egyFont(t)
	got := Shape(f.NewFace(100), ctrl(egBeginSeg)+"A"+ctrl(egHJoin)+"B"+ctrl(egEndSeg), Options{Script: "egyp"})
	if len(got) != 2 {
		t.Fatalf("(A*B) -> %d glyphs, want 2: %+v", len(got), got)
	}
}

// TestEgyEnclosure wraps a sign in an enclosure: it is inset (scaled below 1).
func TestEgyEnclosure(t *testing.T) {
	f := egyFont(t)
	for _, pair := range [][2]rune{{egBeginEnc, egEndEnc}, {egBeginWEnc, egEndWEnc}} {
		got := Shape(f.NewFace(100), ctrl(pair[0])+"A"+ctrl(pair[1]), Options{Script: "egyp"})
		if len(got) != 1 {
			t.Fatalf("enclosure -> %d glyphs, want 1: %+v", len(got), got)
		}
		if !(got[0].Scale < 1) {
			t.Errorf("enclosed sign not inset: scale %v", got[0].Scale)
		}
	}
}

// TestEgyMultiQuadrat: adjacent signs with no joiner are separate quadrats, laid
// side by side, each carrying its own block advance.
func TestEgyMultiQuadrat(t *testing.T) {
	f := egyFont(t)
	got := Shape(f.NewFace(100), "AB", Options{Script: "egyp"})
	if len(got) != 2 {
		t.Fatalf("AB -> %d glyphs, want 2: %+v", len(got), got)
	}
	for _, g := range got {
		if g.Scale != 1 || g.XAdvance == 0 {
			t.Errorf("lone-sign quadrat not full-size/advanced: %+v", g)
		}
	}
}

// TestEgyStrayLeadingJoiner: a run that opens with a joiner has no left operand;
// the empty quadrat produces no glyph and the following sign shapes normally.
func TestEgyStrayLeadingJoiner(t *testing.T) {
	f := egyFont(t)
	got := Shape(f.NewFace(100), ctrl(egVJoin)+"A", Options{Script: "egyp"})
	if len(got) != 1 || got[0].Cluster != 1 {
		t.Fatalf("stray joiner -> %+v, want single glyph for A", got)
	}
}

// TestEgyTrailingJoiner: a trailing joiner leaves a dangling blank operand which
// contributes no glyph.
func TestEgyTrailingJoiner(t *testing.T) {
	f := egyFont(t)
	got := Shape(f.NewFace(100), "A"+ctrl(egVJoin), Options{Script: "egyp"})
	if len(got) != 1 || got[0].Cluster != 0 {
		t.Fatalf("trailing joiner -> %+v, want single glyph for A", got)
	}
}

// TestEgyDanglingBeginGroup: a begin-group with no content or close terminates
// cleanly with no glyphs.
func TestEgyDanglingBeginGroup(t *testing.T) {
	f := egyFont(t)
	got := Shape(f.NewFace(100), ctrl(egBeginSeg), Options{Script: "egyp"})
	if len(got) != 0 {
		t.Fatalf("dangling begin-group -> %+v, want no glyphs", got)
	}
}

// TestEgyAllBlank: a lone blank occupies space but draws nothing.
func TestEgyAllBlank(t *testing.T) {
	f := egyFont(t)
	got := Shape(f.NewFace(100), ctrl(egBlankLo), Options{Script: "egyp"})
	if len(got) != 0 {
		t.Fatalf("blank -> %+v, want no glyphs", got)
	}
}

// TestEgyFontSubstLigatureGuard: a font whose ccmp ligates the two signs must
// not collapse the quadrat; applyFontSubst keeps the original glyphs when the
// count would change, so the vertical-join structure survives.
func TestEgyFontSubstLigatureGuard(t *testing.T) {
	f := ligEgyFont(t)
	got := Shape(f.NewFace(100), "A"+ctrl(egVJoin)+"B", Options{Script: "egyp"})
	if len(got) != 2 {
		t.Fatalf("ligating font collapsed quadrat: %+v", got)
	}
	if got[0].GID != 1 || got[1].GID != 2 {
		t.Errorf("glyphs = %d,%d want 1,2 (ligature suppressed)", got[0].GID, got[1].GID)
	}
}

// TestEgyFontSubstEmptyQuadrat: applyFontSubst is a no-op on a quadrat with no
// signs even when the font has a GSUB.
func TestEgyFontSubstEmptyQuadrat(t *testing.T) {
	f := ligEgyFont(t)
	if got := Shape(f.NewFace(100), ctrl(egBlankLo), Options{Script: "egyp"}); len(got) != 0 {
		t.Fatalf("blank quadrat with GSUB -> %+v, want none", got)
	}
}

// TestRealEgyptianQuadrat validates quadrat layout on Noto Sans Egyptian
// Hieroglyphs: a two-sign vertical-joiner group stacks (both scaled, second
// below first), unlike the same two signs as a plain row (full size, no stack).
func TestRealEgyptianQuadrat(t *testing.T) {
	f, err := opentype.Parse(notosansegyptianhieroglyphs.TTF)
	if err != nil {
		t.Fatalf("parse Noto Sans Egyptian Hieroglyphs: %v", err)
	}
	face := f.NewFace(128)

	stacked := Shape(face, ctrl(0x13000)+ctrl(egVJoin)+ctrl(0x13001), Options{})
	if len(stacked) != 2 {
		t.Fatalf("stacked quadrat -> %d glyphs, want 2: %+v", len(stacked), stacked)
	}
	for _, g := range stacked {
		if !(g.Scale < 1) {
			t.Errorf("stacked sign not scaled: %+v", g)
		}
		if g.GID == 0 {
			t.Errorf("sign unmapped in Noto: %+v", g)
		}
	}
	if !(stacked[0].YOffset > stacked[1].YOffset) {
		t.Errorf("second sign not below first: %+v", stacked)
	}

	row := Shape(face, ctrl(0x13000)+ctrl(0x13001), Options{})
	if len(row) != 2 {
		t.Fatalf("row -> %d glyphs, want 2: %+v", len(row), row)
	}
	for _, g := range row {
		if g.Scale != 1 {
			t.Errorf("row sign scaled down unexpectedly: %+v", g)
		}
	}
	if stacked[0].Scale >= row[0].Scale {
		t.Errorf("stacking did not scale signs smaller than a row")
	}
	t.Logf("stacked=%+v\nrow=%+v", stacked, row)
}
