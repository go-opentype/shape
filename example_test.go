// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape_test

import (
	"fmt"

	"github.com/go-opentype/bidi"
	"github.com/go-opentype/fonts/goregular"
	"github.com/go-opentype/fonts/notosansarabic"
	"github.com/go-opentype/opentype"
	"github.com/go-opentype/shape"
)

// ExampleShape shapes an Arabic word, بيت ("house"): beh-yeh-teh. Noto Sans
// Arabic decomposes each letter into a rasm skeleton plus dot marks via
// GSUB ccmp, so the shaper emits two glyphs (rasm + dots) per letter — six
// glyphs for three letters. Bidi reordering also runs: the run is emitted
// left-to-right in drawing order even though the source text is
// right-to-left, so the glyphs for the last logical letter (Cluster 2, teh)
// are emitted first.
func ExampleShape() {
	f, err := opentype.Parse(notosansarabic.TTF)
	if err != nil {
		panic(err)
	}
	face := f.NewFace(32)

	glyphs := shape.Shape(face, "بيت", shape.Options{})
	fmt.Println("glyph count:", len(glyphs))
	fmt.Println("first glyph's source rune (cluster):", glyphs[0].Cluster)
	fmt.Println("last glyph's source rune (cluster):", glyphs[len(glyphs)-1].Cluster)
	// Output:
	// glyph count: 6
	// first glyph's source rune (cluster): 2
	// last glyph's source rune (cluster): 0
}

// ExampleShape_latin shapes plain Latin text with the default shaper: one
// glyph per rune, left-to-right, kerned and advanced by GPOS/hmtx.
func ExampleShape_latin() {
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		panic(err)
	}
	face := f.NewFace(32)

	glyphs := shape.Shape(face, "Wave", shape.Options{})
	total := 0
	for _, g := range glyphs {
		total += g.XAdvance
	}
	fmt.Println("glyph count:", len(glyphs))
	fmt.Println("total advance:", total)
	// Output:
	// glyph count: 4
	// total advance: 82
}

// ExampleOptions forces the script and base direction instead of relying on
// auto-detection, useful when the caller already knows the text's script
// (for example, from higher-level document metadata).
func ExampleOptions() {
	f, err := opentype.Parse(notosansarabic.TTF)
	if err != nil {
		panic(err)
	}
	face := f.NewFace(32)

	opts := shape.Options{
		Script:    "arab",
		Direction: bidi.RightToLeft,
	}
	glyphs := shape.Shape(face, "بيت", opts)
	fmt.Println("glyph count:", len(glyphs))
	// Output: glyph count: 6
}
