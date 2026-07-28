// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"encoding/binary"
	"sort"
	"testing"

	"github.com/go-opentype/opentype"
)

// This file assembles a minimal, layout-table-free TrueType font in memory so
// the shaper's no-GSUB / no-GPOS code paths are exercised deterministically
// (no real font ships without those tables). The glyphs are empty outlines; the
// shaper never rasterises, it only reads the cmap and horizontal metrics.

type bw struct{ b []byte }

func (w *bw) u16(v uint16) { w.b = binary.BigEndian.AppendUint16(w.b, v) }
func (w *bw) u32(v uint32) { w.b = binary.BigEndian.AppendUint32(w.b, v) }
func (w *bw) i16(v int16)  { w.u16(uint16(v)) }

// assemble builds an sfnt container from the given tables. Checksums are left
// zero (opentype.Parse does not validate them).
func assemble(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for t := range tables {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	headerLen := 12 + 16*len(tags)
	var body []byte
	type rec struct {
		tag       string
		off, size int
	}
	recs := make([]rec, 0, len(tags))
	off := headerLen
	for _, tag := range tags {
		d := tables[tag]
		recs = append(recs, rec{tag, off, len(d)})
		body = append(body, d...)
		off += len(d)
	}
	w := &bw{}
	w.u32(0x00010000) // TrueType sfnt version
	w.u16(uint16(len(tags)))
	w.u16(0) // searchRange
	w.u16(0) // entrySelector
	w.u16(0) // rangeShift
	for _, r := range recs {
		w.b = append(w.b, []byte(r.tag)...)
		w.u32(0)              // checksum
		w.u32(uint32(r.off))  // offset
		w.u32(uint32(r.size)) // length
	}
	w.b = append(w.b, body...)
	return w.b
}

func headTable(upem int, locFmt int16) []byte {
	w := &bw{}
	w.u32(0x00010000) // version
	w.u32(0)          // fontRevision
	w.u32(0)          // checkSumAdjustment
	w.u32(0x5F0F3CF5) // magicNumber
	w.u16(0)          // flags
	w.u16(uint16(upem))
	w.u32(0)
	w.u32(0) // created
	w.u32(0)
	w.u32(0) // modified
	w.i16(0)
	w.i16(0)
	w.i16(0)
	w.i16(0) // bbox
	w.u16(0) // macStyle
	w.u16(0) // lowestRecPPEM
	w.i16(0) // fontDirectionHint
	w.i16(locFmt)
	w.i16(0) // glyphDataFormat
	return w.b
}

func maxpTable(numGlyphs int) []byte {
	w := &bw{}
	w.u32(0x00010000)
	w.u16(uint16(numGlyphs))
	for i := 0; i < 13; i++ {
		w.u16(0) // remaining version-1.0 fields
	}
	return w.b
}

func hheaTable(asc, desc, gap, numH int) []byte {
	w := &bw{}
	w.u32(0x00010000)
	w.i16(int16(asc))
	w.i16(int16(desc))
	w.i16(int16(gap))
	w.u16(0) // advanceWidthMax
	w.i16(0)
	w.i16(0)
	w.i16(0) // min LSB, min RSB, xMaxExtent
	w.i16(1)
	w.i16(0)
	w.i16(0) // caret slope rise/run/offset
	w.i16(0)
	w.i16(0)
	w.i16(0)
	w.i16(0) // 4 reserved
	w.i16(0) // metricDataFormat
	w.u16(uint16(numH))
	return w.b
}

func hmtxTable(adv, lsb []int) []byte {
	w := &bw{}
	for i := range adv {
		w.u16(uint16(adv[i]))
		w.i16(int16(lsb[i]))
	}
	return w.b
}

// cmap4 builds a format-4 cmap subtable, one segment per rune.
func cmap4(m map[rune]uint16) []byte {
	rs := make([]int, 0, len(m))
	for r := range m {
		rs = append(rs, int(r))
	}
	sort.Ints(rs)
	var end, start []uint16
	var delta []int16
	for _, r := range rs {
		end = append(end, uint16(r))
		start = append(start, uint16(r))
		delta = append(delta, int16(int(m[rune(r)])-r))
	}
	end = append(end, 0xFFFF)
	start = append(start, 0xFFFF)
	delta = append(delta, 1)
	seg := len(end)
	w := &bw{}
	w.u16(4)
	w.u16(uint16(16 + 8*seg)) // length
	w.u16(0)                  // language
	w.u16(uint16(seg * 2))    // segCountX2
	w.u16(0)                  // searchRange
	w.u16(0)                  // entrySelector
	w.u16(0)                  // rangeShift
	for _, e := range end {
		w.u16(e)
	}
	w.u16(0) // reservedPad
	for _, s := range start {
		w.u16(s)
	}
	for _, d := range delta {
		w.i16(d)
	}
	for range end {
		w.u16(0) // idRangeOffset
	}
	return w.b
}

func cmapTable(sub []byte) []byte {
	w := &bw{}
	w.u16(0) // version
	w.u16(1) // numTables
	w.u16(3) // platformID (Windows)
	w.u16(1) // encodingID (BMP)
	w.u32(12)
	w.b = append(w.b, sub...)
	return w.b
}

// bareFont assembles a TrueType font with a cmap and horizontal metrics but no
// GSUB, GPOS or kern table. adv overrides the default 500-unit advance per rune.
func bareFont(t *testing.T, m map[rune]uint16, upem int, adv map[rune]int) *opentype.Font {
	t.Helper()
	maxGid := 0
	for _, g := range m {
		if int(g) > maxGid {
			maxGid = int(g)
		}
	}
	numGlyphs := maxGid + 1
	advances := make([]int, numGlyphs)
	lsbs := make([]int, numGlyphs)
	for i := range advances {
		advances[i] = 500
	}
	for r, g := range m {
		if a, ok := adv[r]; ok {
			advances[g] = a
		}
	}
	loca := &bw{}
	for i := 0; i <= numGlyphs; i++ {
		loca.u16(0) // all-empty glyphs
	}
	tables := map[string][]byte{
		"head": headTable(upem, 0),
		"maxp": maxpTable(numGlyphs),
		"hhea": hheaTable(800, -200, 100, numGlyphs),
		"hmtx": hmtxTable(advances, lsbs),
		"cmap": cmapTable(cmap4(m)),
		"loca": loca.b,
		"glyf": {},
	}
	f, err := opentype.Parse(assemble(tables))
	if err != nil {
		t.Fatalf("parse bare font: %v", err)
	}
	return f
}

// TestBareFontNoLayout drives the no-GSUB / no-GPOS path: substitution and
// positioning are skipped, advances come straight from hmtx, offsets are zero,
// and the run is left-to-right.
func TestBareFontNoLayout(t *testing.T) {
	f := bareFont(t, map[rune]uint16{'A': 1, 'B': 2}, 1000, map[rune]int{'A': 600, 'B': 400})
	if f.GSUB() != nil || f.GPOS() != nil {
		t.Fatal("bare font unexpectedly has layout tables")
	}
	face := f.NewFace(500) // scale 0.5
	got := Shape(face, "AB", Options{})
	if len(got) != 2 {
		t.Fatalf("AB -> %d glyphs, want 2: %+v", len(got), got)
	}
	if got[0].Cluster != 0 || got[1].Cluster != 1 {
		t.Errorf("clusters = %d,%d want 0,1", got[0].Cluster, got[1].Cluster)
	}
	if got[0].XAdvance != 300 || got[1].XAdvance != 200 {
		t.Errorf("advances = %d,%d want 300,200", got[0].XAdvance, got[1].XAdvance)
	}
	for _, g := range got {
		if g.XOffset != 0 || g.YOffset != 0 || g.YAdvance != 0 {
			t.Errorf("bare font produced non-zero positioning: %+v", g)
		}
	}
}

// TestEmptyText returns no glyphs for empty input.
func TestEmptyText(t *testing.T) {
	f := bareFont(t, map[rune]uint16{'A': 1}, 1000, nil)
	if got := Shape(f.NewFace(16), "", Options{}); got != nil {
		t.Errorf("Shape(\"\") = %v, want nil", got)
	}
}
