// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import "testing"

// TestVerticalSwapStackVORG: with a vert GSUB feature plus vhea/vmtx/VORG, a
// two-ideograph run is laid top to bottom — each glyph swapped to its vertical
// form, carrying a non-zero vertical advance, centred horizontally, and offset
// from its VORG vertical origin.
func TestVerticalSwapStackVORG(t *testing.T) {
	f := vertFont(t, true)
	face := f.NewFace(100) // scale 0.1
	const s = "一一"

	horiz := Shape(face, s, Options{})
	if horiz[0].GID != 1 {
		t.Fatalf("horizontal form GID = %d, want 1", horiz[0].GID)
	}

	got := Shape(face, s, Options{Vertical: true})
	if len(got) != 2 {
		t.Fatalf("vertical -> %d glyphs, want 2: %+v", len(got), got)
	}
	for i, g := range got {
		if g.GID != 3 {
			t.Errorf("glyph %d GID = %d, want 3 (vert form)", i, g.GID)
		}
		if g.YAdvance != 100 { // vmtx 1000 * 0.1
			t.Errorf("glyph %d YAdvance = %d, want 100", i, g.YAdvance)
		}
		if g.XOffset != -45 { // -(hadv 900 * 0.1)/2
			t.Errorf("glyph %d XOffset = %d, want -45 (centred)", i, g.XOffset)
		}
		if g.YOffset != 88 { // VORG 880 * 0.1
			t.Errorf("glyph %d YOffset = %d, want 88 (VORG)", i, g.YOffset)
		}
		if g.Cluster != i {
			t.Errorf("glyph %d cluster = %d", i, g.Cluster)
		}
	}
}

// TestVerticalNoVORG: without a VORG table the vertical origin falls back to the
// vhea ascender, and the vert feature still swaps forms.
func TestVerticalNoVORG(t *testing.T) {
	f := vertFont(t, false)
	got := Shape(f.NewFace(100), "一", Options{Vertical: true})
	if len(got) != 1 {
		t.Fatalf("-> %d glyphs, want 1: %+v", len(got), got)
	}
	if got[0].GID != 3 {
		t.Errorf("GID = %d, want 3 (vert form)", got[0].GID)
	}
	if got[0].YOffset != 88 { // vhea ascender 880 * 0.1
		t.Errorf("YOffset = %d, want 88 (vhea ascender)", got[0].YOffset)
	}
}

// TestVerticalBareFont: a font with no GSUB and no vertical tables still lays
// out top to bottom — glyph unchanged, vertical advance falls back to one em,
// and the vertical origin is zero.
func TestVerticalBareFont(t *testing.T) {
	f := bareFont(t, map[rune]uint16{'A': 1}, 1000, map[rune]int{'A': 600})
	got := Shape(f.NewFace(100), "A", Options{Vertical: true})
	if len(got) != 1 {
		t.Fatalf("-> %d glyphs, want 1: %+v", len(got), got)
	}
	g := got[0]
	if g.GID != 1 {
		t.Errorf("GID = %d, want 1 (no substitution)", g.GID)
	}
	if g.YAdvance != 100 { // one em (1000) * 0.1
		t.Errorf("YAdvance = %d, want 100 (em fallback)", g.YAdvance)
	}
	if g.YOffset != 0 {
		t.Errorf("YOffset = %d, want 0 (no vhea/VORG)", g.YOffset)
	}
	if g.XOffset != -30 { // -(hadv 600 * 0.1)/2
		t.Errorf("XOffset = %d, want -30 (centred)", g.XOffset)
	}
}
