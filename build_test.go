// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"testing"

	"github.com/go-opentype/opentype"
)

// This file assembles in-memory fonts carrying the layout tables the Egyptian
// and vertical paths exercise: a GSUB (single-substitution for vert, and a
// ligature for the ccmp count-change case) and the vertical metrics tables
// vhea/vmtx/VORG. The binary layout mirrors the OpenType spec; the builders are
// deliberately minimal (single script "DFLT", version 1.0 tables). It builds on
// the sfnt assembler and primitive writers in synth_test.go.

// --- GSUB script/feature/lookup builders ------------------------------------

type tLangSys struct {
	required uint16
	features []uint16
}

type tScript struct {
	tag string
	def *tLangSys
}

type tFeature struct {
	tag     string
	lookups []uint16
}

func buildLangSys(ls tLangSys) []byte {
	w := &bw{}
	w.u16(0) // lookupOrderOffset
	w.u16(ls.required)
	w.u16(uint16(len(ls.features)))
	for _, f := range ls.features {
		w.u16(f)
	}
	return w.b
}

func buildScript(s tScript) []byte {
	header := 4
	var body []byte
	defOff := 0
	if s.def != nil {
		defOff = header
		body = append(body, buildLangSys(*s.def)...)
	}
	w := &bw{}
	w.u16(uint16(defOff))
	w.u16(0) // langSysCount
	w.b = append(w.b, body...)
	return w.b
}

func buildScriptList(scripts []tScript) []byte {
	header := 2 + 6*len(scripts)
	var body []byte
	type srec struct {
		tag string
		off int
	}
	var srecs []srec
	for _, s := range scripts {
		off := header + len(body)
		body = append(body, buildScript(s)...)
		srecs = append(srecs, srec{s.tag, off})
	}
	w := &bw{}
	w.u16(uint16(len(scripts)))
	for _, sr := range srecs {
		w.b = append(w.b, []byte(sr.tag)...)
		w.u16(uint16(sr.off))
	}
	w.b = append(w.b, body...)
	return w.b
}

func buildFeature(f tFeature) []byte {
	w := &bw{}
	w.u16(0) // featureParamsOffset
	w.u16(uint16(len(f.lookups)))
	for _, l := range f.lookups {
		w.u16(l)
	}
	return w.b
}

func buildFeatureList(feats []tFeature) []byte {
	header := 2 + 6*len(feats)
	var body []byte
	type frec struct {
		tag string
		off int
	}
	var frecs []frec
	for _, f := range feats {
		off := header + len(body)
		body = append(body, buildFeature(f)...)
		frecs = append(frecs, frec{f.tag, off})
	}
	w := &bw{}
	w.u16(uint16(len(feats)))
	for _, fr := range frecs {
		w.b = append(w.b, []byte(fr.tag)...)
		w.u16(uint16(fr.off))
	}
	w.b = append(w.b, body...)
	return w.b
}

func buildLookupList(lookups [][]byte) []byte {
	header := 2 + 2*len(lookups)
	var body []byte
	var offs []int
	for _, lk := range lookups {
		offs = append(offs, header+len(body))
		body = append(body, lk...)
	}
	w := &bw{}
	w.u16(uint16(len(lookups)))
	for _, o := range offs {
		w.u16(uint16(o))
	}
	w.b = append(w.b, body...)
	return w.b
}

func buildLookup(lookupType uint16, subtables [][]byte) []byte {
	header := 6 + 2*len(subtables)
	var body []byte
	var offs []int
	for _, st := range subtables {
		offs = append(offs, header+len(body))
		body = append(body, st...)
	}
	w := &bw{}
	w.u16(lookupType)
	w.u16(0) // lookupFlag
	w.u16(uint16(len(subtables)))
	for _, o := range offs {
		w.u16(uint16(o))
	}
	w.b = append(w.b, body...)
	return w.b
}

func buildLayoutTable(scripts []tScript, feats []tFeature, lookups [][]byte) []byte {
	sl := buildScriptList(scripts)
	fl := buildFeatureList(feats)
	ll := buildLookupList(lookups)
	scriptOff := 10
	featureOff := scriptOff + len(sl)
	lookupOff := featureOff + len(fl)
	w := &bw{}
	w.u16(1) // majorVersion
	w.u16(0) // minorVersion
	w.u16(uint16(scriptOff))
	w.u16(uint16(featureOff))
	w.u16(uint16(lookupOff))
	w.b = append(w.b, sl...)
	w.b = append(w.b, fl...)
	w.b = append(w.b, ll...)
	return w.b
}

func buildCoverage1(glyphs ...opentype.GlyphIndex) []byte {
	w := &bw{}
	w.u16(1)
	w.u16(uint16(len(glyphs)))
	for _, g := range glyphs {
		w.u16(uint16(g))
	}
	return w.b
}

// buildSingle1 builds a single-substitution format 1 subtable (a delta added to
// each covered glyph id).
func buildSingle1(delta int16, cov []byte) []byte {
	w := &bw{}
	w.u16(1)
	w.u16(6) // coverage offset (right after the 6-byte header)
	w.i16(delta)
	w.b = append(w.b, cov...)
	return w.b
}

func buildLigature(glyph opentype.GlyphIndex, rest ...opentype.GlyphIndex) []byte {
	w := &bw{}
	w.u16(uint16(glyph))
	w.u16(uint16(len(rest) + 1)) // componentCount includes the first component
	for _, c := range rest {
		w.u16(uint16(c))
	}
	return w.b
}

func buildLigatureSet(ligs [][]byte) []byte {
	header := 2 + 2*len(ligs)
	var body []byte
	var offs []int
	for _, l := range ligs {
		offs = append(offs, header+len(body))
		body = append(body, l...)
	}
	w := &bw{}
	w.u16(uint16(len(ligs)))
	for _, o := range offs {
		w.u16(uint16(o))
	}
	w.b = append(w.b, body...)
	return w.b
}

func buildLigatureSubst(cov []byte, sets [][]byte) []byte {
	header := 6 + 2*len(sets)
	var body []byte
	var setOffs []int
	for _, s := range sets {
		setOffs = append(setOffs, header+len(body))
		body = append(body, s...)
	}
	covOff := header + len(body)
	w := &bw{}
	w.u16(1)
	w.u16(uint16(covOff))
	w.u16(uint16(len(sets)))
	for _, o := range setOffs {
		w.u16(uint16(o))
	}
	w.b = append(w.b, body...)
	w.b = append(w.b, cov...)
	return w.b
}

// --- vertical metrics table builders ----------------------------------------

// vheaTable builds a version-1.0 vhea: ascender/descender/lineGap at offsets
// 4/6/8 and numOfLongVerMetrics (uint16) at offset 34, in a 36-byte table.
func vheaTable(asc, desc, gap, numVMetrics int) []byte {
	w := &bw{}
	w.u32(0x00010000) // version
	w.i16(int16(asc))
	w.i16(int16(desc))
	w.i16(int16(gap))
	for len(w.b) < 34 {
		w.b = append(w.b, 0)
	}
	w.u16(uint16(numVMetrics))
	return w.b
}

// vmtxTable builds a vmtx with numOfLongVerMetrics == len(adv) (advanceHeight,
// topSideBearing) pairs.
func vmtxTable(adv, tsb []int) []byte {
	w := &bw{}
	for i := range adv {
		w.u16(uint16(adv[i]))
		w.i16(int16(tsb[i]))
	}
	return w.b
}

// vorgTable builds a VORG carrying only a default vertical origin (no per-glyph
// overrides).
func vorgTable(defaultY int) []byte {
	w := &bw{}
	w.u16(1) // majorVersion
	w.u16(0) // minorVersion
	w.i16(int16(defaultY))
	w.u16(0) // numVertOriginYMetrics
	return w.b
}

// --- assembled fonts --------------------------------------------------------

// baseTables returns the mandatory non-layout tables for a font with the given
// cmap, advances and top side bearings, at 1000 units/em.
func baseTables(m map[rune]uint16, numGlyphs int, adv, lsb []int) map[string][]byte {
	loca := &bw{}
	for i := 0; i <= numGlyphs; i++ {
		loca.u16(0)
	}
	return map[string][]byte{
		"head": headTable(1000, 0),
		"maxp": maxpTable(numGlyphs),
		"hhea": hheaTable(800, -200, 100, numGlyphs),
		"hmtx": hmtxTable(adv, lsb),
		"cmap": cmapTable(cmap4(m)),
		"loca": loca.b,
		"glyf": {},
	}
}

// vertGSUB builds a GSUB whose vert feature maps glyph 1 to glyph 3 (delta +2)
// under the DFLT script.
func vertGSUB() []byte {
	single := buildLookup(1, [][]byte{buildSingle1(2, buildCoverage1(1))})
	scripts := []tScript{{tag: "DFLT", def: &tLangSys{required: 0xFFFF, features: []uint16{0}}}}
	feats := []tFeature{{tag: "vert", lookups: []uint16{0}}}
	return buildLayoutTable(scripts, feats, [][]byte{single})
}

// vertFont assembles a CJK-like font with vhea/vmtx and a vert GSUB feature.
// withVORG adds a VORG default origin. Glyph 1 is the horizontal form of the
// mapped rune, glyph 3 its vertical (vert) form.
func vertFont(t *testing.T, withVORG bool) *opentype.Font {
	t.Helper()
	const rune0 = 0x4E00 // CJK ideograph, mapped to glyph 1
	numGlyphs := 4
	adv := []int{500, 900, 500, 900}
	lsb := []int{0, 0, 0, 0}
	tables := baseTables(map[rune]uint16{rune0: 1}, numGlyphs, adv, lsb)
	tables["GSUB"] = vertGSUB()
	tables["vhea"] = vheaTable(880, -120, 0, numGlyphs)
	tables["vmtx"] = vmtxTable([]int{1000, 1000, 1000, 1000}, []int{0, 0, 0, 0})
	if withVORG {
		tables["VORG"] = vorgTable(880)
	}
	f, err := opentype.Parse(assemble(tables))
	if err != nil {
		t.Fatalf("parse vertical font: %v", err)
	}
	return f
}

// ligEgyFont assembles a font whose GSUB ccmp feature ligates glyphs 1+2 into
// glyph 3, so a two-sign quadrat exercises the count-change guard in
// applyFontSubst. Runes 'A' and 'B' map to glyphs 1 and 2.
func ligEgyFont(t *testing.T) *opentype.Font {
	t.Helper()
	numGlyphs := 4
	adv := []int{500, 600, 700, 500}
	lsb := []int{0, 0, 0, 0}
	tables := baseTables(map[rune]uint16{'A': 1, 'B': 2}, numGlyphs, adv, lsb)
	set := buildLigatureSet([][]byte{buildLigature(3, 2)}) // 1,2 -> 3
	lig := buildLookup(4, [][]byte{buildLigatureSubst(buildCoverage1(1), [][]byte{set})})
	scripts := []tScript{{tag: "DFLT", def: &tLangSys{required: 0xFFFF, features: []uint16{0}}}}
	feats := []tFeature{{tag: "ccmp", lookups: []uint16{0}}}
	tables["GSUB"] = buildLayoutTable(scripts, feats, [][]byte{lig})
	f, err := opentype.Parse(assemble(tables))
	if err != nil {
		t.Fatalf("parse ligature font: %v", err)
	}
	return f
}
