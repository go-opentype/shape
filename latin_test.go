// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"testing"

	"github.com/go-opentype/fonts/lora"
	"github.com/go-opentype/opentype"
)

func loraFont(t *testing.T) *opentype.Font {
	t.Helper()
	f, err := opentype.Parse(lora.TTF)
	if err != nil {
		t.Fatalf("parse Lora: %v", err)
	}
	return f
}

// TestLatinLigature drives the default (non-Arabic) path: "fi" collapses to a
// single ligature glyph via the liga feature, and the ligature keeps its first
// component's cluster.
func TestLatinLigature(t *testing.T) {
	f := loraFont(t)
	face := f.NewFace(64)
	got := Shape(face, "fi", Options{})
	if len(got) != 1 {
		t.Fatalf("fi -> %d glyphs, want 1 (ligature): %+v", len(got), got)
	}
	fg, _ := f.GlyphIndex('f')
	ig, _ := f.GlyphIndex('i')
	if got[0].GID == fg || got[0].GID == ig {
		t.Errorf("ligature GID %d equals a component (f=%d i=%d)", got[0].GID, fg, ig)
	}
	if got[0].Cluster != 0 {
		t.Errorf("ligature cluster = %d, want 0", got[0].Cluster)
	}
}

// TestLatinLTROrderAndPositions shapes a plain Latin word: it stays in logical
// (left-to-right) order, clusters are ascending, and GPOS advances are applied
// (positions are non-nil for a font with a GPOS table).
func TestLatinLTROrderAndPositions(t *testing.T) {
	f := loraFont(t)
	face := f.NewFace(48)
	got := Shape(face, "Lo", Options{Script: "latn", Features: []string{"kern"}})
	if len(got) != 2 {
		t.Fatalf("Lo -> %d glyphs, want 2: %+v", len(got), got)
	}
	if got[0].Cluster != 0 || got[1].Cluster != 1 {
		t.Errorf("clusters = %d,%d want 0,1 (LTR): %+v", got[0].Cluster, got[1].Cluster, got)
	}
	for _, g := range got {
		if g.XAdvance <= 0 {
			t.Errorf("non-positive advance: %+v", g)
		}
	}
}
