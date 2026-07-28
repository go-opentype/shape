// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"testing"

	"github.com/go-opentype/fonts/inter"
	"github.com/go-opentype/opentype"
)

func eqRunes(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestHasHangulJamo(t *testing.T) {
	if !hasHangulJamo([]rune{'a', 0x1100, 'b'}) {
		t.Fatal("want true when a conjoining jamo is present")
	}
	if hasHangulJamo([]rune("abc가")) { // precomposed 가 is NOT a conjoining jamo
		t.Fatal("want false for Latin + a precomposed syllable")
	}
}

func TestComposeHangulFastPath(t *testing.T) {
	in := []rune("hello 가나다")
	out := composeHangul(in)
	if !eqRunes(in, out) {
		t.Fatalf("no conjoining jamo: composeHangul changed the run: %q", string(out))
	}
}

func TestComposeHangulLV(t *testing.T) {
	// ㄱ(U+1100) + ㅏ(U+1161) -> 가(U+AC00)
	out := composeHangul([]rune{0x1100, 0x1161})
	if !eqRunes(out, []rune{0xAC00}) {
		t.Fatalf("L+V compose = %U, want [U+AC00]", out)
	}
}

func TestComposeHangulLVT(t *testing.T) {
	// ㄱ + ㅏ + ㄱ(U+11A8) -> 각(U+AC01)
	out := composeHangul([]rune{0x1100, 0x1161, 0x11A8})
	if !eqRunes(out, []rune{0xAC01}) {
		t.Fatalf("L+V+T compose = %U, want [U+AC01]", out)
	}
	// ㅎ+ㅏ+ㄴ -> 한(U+D55C)
	han := composeHangul([]rune{0x1112, 0x1161, 0x11AB})
	if !eqRunes(han, []rune{0xD55C}) {
		t.Fatalf("한 compose = %U, want [U+D55C]", han)
	}
}

func TestComposeHangulLVSyllablePlusT(t *testing.T) {
	// Already-composed 가(U+AC00, T-less) + ㄱ(U+11A8) -> 각(U+AC01).
	out := composeHangul([]rune{0xAC00, 0x11A8})
	if !eqRunes(out, []rune{0xAC01}) {
		t.Fatalf("LV-syllable+T = %U, want [U+AC01]", out)
	}
	// A syllable that already carries a T (각, %28 != 0) does NOT absorb another T.
	noAbsorb := composeHangul([]rune{0xAC01, 0x11A8})
	if !eqRunes(noAbsorb, []rune{0xAC01, 0x11A8}) {
		t.Fatalf("LVT+T should not compose, got %U", noAbsorb)
	}
}

func TestComposeHangulNonComposing(t *testing.T) {
	// V with no preceding L (out empty on first, then no L before V).
	if out := composeHangul([]rune{0x1161}); !eqRunes(out, []rune{0x1161}) {
		t.Fatalf("lone V = %U, want unchanged", out)
	}
	// L followed by another L (not a V) — both kept.
	if out := composeHangul([]rune{0x1100, 0x1100}); !eqRunes(out, []rune{0x1100, 0x1100}) {
		t.Fatalf("L+L = %U, want unchanged", out)
	}
	// LV syllable followed by a non-T rune — kept as two.
	if out := composeHangul([]rune{0xAC00, 'x'}); !eqRunes(out, []rune{0xAC00, 'x'}) {
		t.Fatalf("LV+non-T = %U, want unchanged", out)
	}
	// A lone trailing jamo with nothing before it.
	if out := composeHangul([]rune{0x11A8}); !eqRunes(out, []rune{0x11A8}) {
		t.Fatalf("lone T = %U, want unchanged", out)
	}
}

// Shape runs composeHangul as a pre-pass: three conjoining jamo for 한 collapse
// to one syllable, so exactly one glyph is emitted (any font: the point is the
// composition, not which glyph maps).
func TestShapeComposesHangulJamo(t *testing.T) {
	f, err := opentype.Parse(inter.TTF)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	jamo := string([]rune{0x1112, 0x1161, 0x11AB}) // decomposed 한 (한)
	got := Shape(f.NewFace(64), jamo, Options{})
	if len(got) != 1 {
		t.Fatalf("Shape of 3 decomposed jamo = %d glyphs, want 1 (composed)", len(got))
	}
}
