// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"testing"

	"github.com/go-opentype/fonts/notosansarabic"
	"github.com/go-opentype/opentype"
)

func notoArabic(t *testing.T) *opentype.Font {
	t.Helper()
	f, err := opentype.Parse(notosansarabic.TTF)
	if err != nil {
		t.Fatalf("parse Noto Sans Arabic: %v", err)
	}
	return f
}

// gidSet returns the set of glyph ids a shaped run uses.
func gidSet(gs []Glyph) map[opentype.GlyphIndex]bool {
	m := map[opentype.GlyphIndex]bool{}
	for _, g := range gs {
		m[g.GID] = true
	}
	return m
}

// TestRealArabicJoined shapes بيت (beh-yeh-teh) and asserts cursive joining
// actually happened: the joined word uses positional (initial/medial/final)
// glyphs that do not appear when the same letters are shaped in isolation.
// Noto Sans Arabic decomposes each letter into a rasm skeleton + dot marks via
// ccmp, so joining changes the rasm glyphs (initial/medial/final forms) rather
// than mapping one glyph per letter.
func TestRealArabicJoined(t *testing.T) {
	f := notoArabic(t)
	face := f.NewFace(64)

	const word = "بيت" // U+0628 U+064A U+062A
	joined := Shape(face, word, Options{})
	if len(joined) < 3 {
		t.Fatalf("shaped %d glyphs, want >= 3: %+v", len(joined), joined)
	}
	// Clusters must all be valid source indices, and every source rune covered.
	seen := map[int]bool{}
	for _, g := range joined {
		if g.Cluster < 0 || g.Cluster > 2 {
			t.Errorf("glyph cluster %d out of range: %+v", g.Cluster, g)
		}
		seen[g.Cluster] = true
	}
	for c := 0; c < 3; c++ {
		if !seen[c] {
			t.Errorf("source rune %d not covered by any cluster", c)
		}
	}

	// Isolated shaping of each letter on its own.
	isolated := map[opentype.GlyphIndex]bool{}
	for _, r := range []string{"ب", "ي", "ت"} {
		for g := range gidSet(Shape(face, r, Options{})) {
			isolated[g] = true
		}
	}
	// Joining must introduce at least one positional-form glyph absent from the
	// isolated shaping.
	var extra []opentype.GlyphIndex
	for g := range gidSet(joined) {
		if !isolated[g] {
			extra = append(extra, g)
		}
	}
	if len(extra) == 0 {
		t.Errorf("no positional glyphs beyond isolated forms; joining did not occur\njoined=%+v", joined)
	}
	t.Logf("بيت joined=%+v\nnew positional glyphs vs isolated: %v", joined, extra)
}

// TestRealArabicMarkAttached shapes beh + fatha (U+0628 U+064E) and asserts the
// mark (fatha, source cluster 1) is lifted onto its base by GPOS: a non-zero
// YOffset.
func TestRealArabicMarkAttached(t *testing.T) {
	f := notoArabic(t)
	face := f.NewFace(128)
	got := Shape(face, "بَ", Options{})
	if len(got) < 2 {
		t.Fatalf("shaped %d glyphs, want >= 2: %+v", len(got), got)
	}
	attached := false
	for _, g := range got {
		if g.Cluster == 1 && g.YOffset != 0 {
			attached = true
		}
	}
	if !attached {
		t.Errorf("fatha (cluster 1) has no non-zero YOffset; mark not attached: %+v", got)
	}
	t.Logf("beh+fatha = %+v", got)
}

// TestRealArabicVisualOrder asserts the Arabic run is emitted right-to-left:
// the glyph carrying the first logical letter (beh, cluster 0) appears at the
// right end (last in the left-to-right visual slice).
func TestRealArabicVisualOrder(t *testing.T) {
	f := notoArabic(t)
	face := f.NewFace(48)
	got := Shape(face, "بيت", Options{})
	// Last visual glyph belongs to the first logical letter (rightmost).
	last := got[len(got)-1]
	if last.Cluster != 0 {
		t.Errorf("rightmost visual glyph cluster = %d, want 0 (RTL order): %+v", last.Cluster, got)
	}
}
