// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package shape

import (
	"reflect"
	"testing"

	"github.com/go-opentype/bidi"
	"github.com/go-opentype/opentype"
)

func TestResolveScript(t *testing.T) {
	cases := []struct {
		want  string
		runes string
		out   string
	}{
		{"arab", "abc", "arab"}, // explicit Arabic
		{"ARAB", "abc", "arab"}, // case-insensitive
		{"", "بيت", "arab"},     // auto-detect Arabic
		{"", "abc", "dflt"},     // auto-detect none -> default
		{"latn", "بيت", "dflt"}, // explicit non-Arabic overrides detection
		{"grek", "abc", "dflt"}, // unknown -> default
	}
	for _, c := range cases {
		if got := resolveScript(c.want, []rune(c.runes)); got != c.out {
			t.Errorf("resolveScript(%q, %q) = %q, want %q", c.want, c.runes, got, c.out)
		}
	}
}

func TestIsArabic(t *testing.T) {
	arabic := []rune{0x0600, 0x0628, 0x06FF, 0x0750, 0x08A0, 0xFB50, 0xFE70, 0xFEFF}
	for _, r := range arabic {
		if !isArabic(r) {
			t.Errorf("isArabic(U+%04X) = false, want true", r)
		}
	}
	notArabic := []rune{'a', 0x05FF, 0x0700, 0x2000, 0xFB49, 0xFF00}
	for _, r := range notArabic {
		if isArabic(r) {
			t.Errorf("isArabic(U+%04X) = true, want false", r)
		}
	}
}

func TestFormMasks(t *testing.T) {
	// clusters map post-ccmp glyphs back to source runes; forms per source rune.
	// runes: [Initial, Medial, Final, Isolated]; a decomposed rune (index 1)
	// yields two glyphs sharing its cluster.
	clusters := []int{0, 1, 1, 2, 3}
	forms := []bidi.JoinForm{bidi.Initial, bidi.Medial, bidi.Final, bidi.Isolated}
	isol, init, medi, fina := formMasks(clusters, forms)
	if want := []bool{true, false, false, false, false}; !reflect.DeepEqual(init, want) {
		t.Errorf("init = %v want %v", init, want)
	}
	if want := []bool{false, true, true, false, false}; !reflect.DeepEqual(medi, want) {
		t.Errorf("medi = %v want %v", medi, want)
	}
	if want := []bool{false, false, false, true, false}; !reflect.DeepEqual(fina, want) {
		t.Errorf("fina = %v want %v", fina, want)
	}
	if want := []bool{false, false, false, false, true}; !reflect.DeepEqual(isol, want) {
		t.Errorf("isol = %v want %v", isol, want)
	}
}

func TestGposFeatures(t *testing.T) {
	if got := gposFeatures("arab", nil); !reflect.DeepEqual(got, []string{"kern", "mark", "mkmk", "curs"}) {
		t.Errorf("arab gpos features = %v", got)
	}
	if got := gposFeatures("dflt", []string{"ss01"}); !reflect.DeepEqual(got, []string{"kern", "mark", "ss01"}) {
		t.Errorf("default gpos features = %v", got)
	}
}

func TestAppendUser(t *testing.T) {
	base := []opentype.FeatureApp{{Tag: "liga"}}
	got := appendUser(base, []string{"ss01", "dlig"})
	if len(got) != 3 || got[1].Tag != "ss01" || got[2].Tag != "dlig" {
		t.Errorf("appendUser = %+v", got)
	}
	// No user features leaves the list unchanged.
	if got := appendUser(base, nil); len(got) != 1 {
		t.Errorf("appendUser(nil) len = %d want 1", len(got))
	}
}

func TestPx(t *testing.T) {
	if got := px(1000, 0.5); got != 500 {
		t.Errorf("px(1000,0.5) = %d want 500", got)
	}
	if got := px(3, 0.5); got != 2 { // 1.5 rounds to 2 (half away from zero)
		t.Errorf("px(3,0.5) = %d want 2", got)
	}
}
