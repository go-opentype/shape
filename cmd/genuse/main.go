// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

// Command genuse generates use_table.go for package shape from the Unicode
// Character Database. It fetches IndicSyllabicCategory.txt,
// IndicPositionalCategory.txt, DerivedGeneralCategory.txt and Scripts.txt,
// derives every relevant code point's Universal Shaping Engine category via
// shape.DeriveUSECategory (the same derivation the package tests exercise),
// coalesces equal-valued runs into ranges, and also emits the code point ranges
// of the USE-handled scripts for auto-detection.
//
// Usage:
//
//	genuse <IndicSyllabicCategory URL> <IndicPositionalCategory URL> \
//	       <DerivedGeneralCategory URL> <Scripts URL> <out.go>
//
// The URLs are ordinarily the UCD files under
// https://www.unicode.org/Public/UCD/latest/ucd/. Keeping them as arguments lets
// the generator run hermetically against a local mirror in tests.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/go-opentype/shape"
)

// Indirection seams so main is coverable without real network or disk.
var (
	osExit  = os.Exit
	httpGet = http.Get
	writeF  = os.WriteFile
)

func main() { osExit(run(os.Args[1:])) }

// run is the generator body. It returns a process exit code: 0 on success, 2 on
// a usage error, 1 on any fetch or write failure.
func run(args []string) int {
	if len(args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: genuse <syllabic URL> <positional URL> <generalCategory URL> <scripts URL> <out.go>")
		return 2
	}
	isc, err := fetchProp(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ipc, err := fetchProp(args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	gc, err := fetchProp(args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sc, err := fetchProp(args[3])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := writeF(args[4], generate(isc, ipc, gc, sc), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// fetchProp downloads a UCD property file from url and parses it.
func fetchProp(url string) (map[rune]string, error) {
	resp, err := httpGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseUCD(body)
}

// parseUCD parses a two-field UCD data file ("<code|range> ; <value> # ...")
// into a per-rune value map, ignoring comments and blank lines.
func parseUCD(data []byte) (map[rune]string, error) {
	out := map[rune]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ";")
		if len(fields) != 2 {
			return nil, fmt.Errorf("bad line %q", line)
		}
		lo, hi, err := parseRange(strings.TrimSpace(fields[0]))
		if err != nil {
			return nil, err
		}
		val := strings.TrimSpace(fields[1])
		for r := lo; r <= hi; r++ {
			out[r] = val
		}
	}
	return out, sc.Err()
}

// parseRange parses "0900" or "0900..0902" into an inclusive rune range.
func parseRange(s string) (lo, hi rune, err error) {
	if a, b, ok := strings.Cut(s, ".."); ok {
		l, err := strconv.ParseInt(a, 16, 32)
		if err != nil {
			return 0, 0, err
		}
		h, err := strconv.ParseInt(b, 16, 32)
		if err != nil {
			return 0, 0, err
		}
		return rune(l), rune(h), nil
	}
	v, err := strconv.ParseInt(s, 16, 32)
	if err != nil {
		return 0, 0, err
	}
	return rune(v), rune(v), nil
}

// overrides are the per-code-point USE categories the specification assigns
// outside the UISC/UIPC/UGC derivation.
var overrides = map[rune]string{
	0x034F: "CGJ",
	0x2060: "WJ",
	0x2015: "GB", 0x2022: "GB",
	0x25FB: "GB", 0x25FC: "GB", 0x25FD: "GB", 0x25FE: "GB",
	0x1A60: "Sk",
	0xFE00: "VS", 0xFE01: "VS", 0xFE02: "VS", 0xFE03: "VS",
	0xFE04: "VS", 0xFE05: "VS", 0xFE06: "VS", 0xFE07: "VS",
	0xFE08: "VS", 0xFE09: "VS", 0xFE0A: "VS", 0xFE0B: "VS",
	0xFE0C: "VS", 0xFE0D: "VS", 0xFE0E: "VS", 0xFE0F: "VS",
}

// useScriptNames is the set of Unicode script names (as spelled in Scripts.txt)
// whose ranges route to the USE shaper. The nine dedicated-shaper Indic scripts
// and Arabic/Latin/etc. are deliberately excluded.
var useScriptNames = map[string]bool{
	"Thai": true, "Lao": true, "Khmer": true, "Myanmar": true, "Tibetan": true,
	"Tai_Tham": true, "Tai_Le": true, "New_Tai_Lue": true, "Tai_Viet": true,
	"Javanese": true, "Balinese": true, "Sundanese": true, "Buginese": true,
	"Batak": true, "Rejang": true, "Cham": true, "Lepcha": true, "Limbu": true,
	"Syloti_Nagri": true, "Saurashtra": true, "Kayah_Li": true,
	"Meetei_Mayek": true, "Chakma": true, "Sharada": true, "Takri": true,
	"Khojki": true, "Khudawadi": true, "Multani": true, "Tirhuta": true,
	"Siddham": true, "Modi": true, "Newa": true, "Grantha": true,
	"Bhaiksuki": true, "Marchen": true, "Masaram_Gondi": true,
	"Gunjala_Gondi": true, "Soyombo": true, "Zanabazar_Square": true,
	"Dogra": true, "Nandinagari": true, "Tagalog": true, "Hanunoo": true,
	"Buhid": true, "Tagbanwa": true, "Phags_Pa": true, "Adlam": true,
	"Hanifi_Rohingya": true, "Mongolian": true, "Sinhala": true, "Ahom": true,
	"Brahmi": true, "Kaithi": true, "Kharoshthi": true, "Mahajani": true,
	"Makasar": true, "Medefaidrin": true, "Miao": true, "Pahawh_Hmong": true,
	"Nyiakeng_Puachue_Hmong": true, "Wancho": true, "Yezidi": true,
	"Chorasmian": true, "Dives_Akuru": true, "Duployan": true, "Elymaic": true,
	"Manichaean": true, "Mandaic": true, "Nko": true, "Old_Uyghur": true,
	"Psalter_Pahlavi": true, "Sogdian": true, "Old_Sogdian": true,
	"Tifinagh": true, "Tangsa": true, "Toto": true, "Vithkuqi": true,
	"Cypro_Minoan": true, "Khitan_Small_Script": true, "Kawi": true,
	"Nag_Mundari": true, "Garay": true, "Gurung_Khema": true, "Kirat_Rai": true,
	"Ol_Onal": true, "Sunuwar": true, "Todhri": true, "Tulu_Tigalari": true,
}

// generate builds the use_table.go source from the four property maps.
func generate(isc, ipc, gc, scripts map[rune]string) []byte {
	// Candidate runes: those with an explicit UISC or UIPC value, plus overrides.
	cand := map[rune]bool{}
	for r := range isc {
		cand[r] = true
	}
	for r := range ipc {
		cand[r] = true
	}
	for r := range overrides {
		cand[r] = true
	}
	runes := make([]rune, 0, len(cand))
	for r := range cand {
		runes = append(runes, r)
	}
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })

	var buf bytes.Buffer
	buf.WriteString("// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.\n")
	buf.WriteString("// Use of this source code is governed by a BSD-3-Clause license that can be\n")
	buf.WriteString("// found in the LICENSE file at the root of this repository.\n\n")
	buf.WriteString("// Code generated by cmd/genuse from the Unicode Character Database; DO NOT EDIT.\n\n")
	buf.WriteString("package shape\n\n")

	buf.WriteString("// useRanges maps Unicode code point ranges to their USE category, sorted by\n")
	buf.WriteString("// Lo. Code points in no range default to ucO. Generated by cmd/genuse.\n")
	buf.WriteString("var useRanges = []useRange{\n")
	emitCats(&buf, runes, func(r rune) string {
		if o, ok := overrides[r]; ok {
			return o
		}
		uisc := isc[r]
		if uisc == "" {
			uisc = "Other"
		}
		return shape.DeriveUSECategory(uisc, ipc[r], gc[r])
	})
	buf.WriteString("}\n\n")

	buf.WriteString("// useScriptRanges lists the code point ranges of the USE-handled scripts,\n")
	buf.WriteString("// sorted by Lo. Generated by cmd/genuse.\n")
	buf.WriteString("var useScriptRanges = []u32range{\n")
	emitScripts(&buf, scripts)
	buf.WriteString("}\n")
	return buf.Bytes()
}

// emitCats writes coalesced classification ranges: consecutive runes sharing a
// non-"O" category collapse into one {lo, hi, ucCat} entry.
func emitCats(buf *bytes.Buffer, runes []rune, catOf func(rune) string) {
	type ent struct {
		lo, hi rune
		cat    string
	}
	var ents []ent
	for _, r := range runes {
		cat := catOf(r)
		if cat == "O" {
			continue
		}
		if n := len(ents); n > 0 && ents[n-1].cat == cat && ents[n-1].hi == r-1 {
			ents[n-1].hi = r
			continue
		}
		ents = append(ents, ent{r, r, cat})
	}
	for _, e := range ents {
		fmt.Fprintf(buf, "\t{0x%04X, 0x%04X, uc%s},\n", e.lo, e.hi, e.cat)
	}
}

// emitScripts writes the coalesced code point ranges of the USE scripts, merging
// adjacent or overlapping ranges.
func emitScripts(buf *bytes.Buffer, scripts map[rune]string) {
	runes := make([]rune, 0, len(scripts))
	for r, name := range scripts {
		if useScriptNames[name] {
			runes = append(runes, r)
		}
	}
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
	type rng struct{ lo, hi rune }
	var out []rng
	for _, r := range runes {
		if n := len(out); n > 0 && r <= out[n-1].hi+1 {
			out[n-1].hi = r
			continue
		}
		out = append(out, rng{r, r})
	}
	for _, e := range out {
		fmt.Fprintf(buf, "\t{0x%04X, 0x%04X},\n", e.lo, e.hi)
	}
}
