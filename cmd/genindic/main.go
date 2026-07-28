// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

// Command genindic generates indictables.go for package shape from the Unicode
// Character Database. It fetches IndicSyllabicCategory.txt and
// IndicPositionalCategory.txt, maps every code point to the shaper's internal
// Indic category and (raw) positional category, coalesces equal-valued runs
// into ranges and writes them as a Go source file.
//
// Usage:
//
//	genindic <IndicSyllabicCategory URL> <IndicPositionalCategory URL> <out.go>
//
// The URLs are ordinarily the UCD files under
// https://www.unicode.org/Public/UCD/latest/ucd/. Keeping them as arguments
// lets the generator run hermetically against a local mirror in tests.
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
)

// Indirection seams so main is coverable without real network or disk.
var (
	osExit  = os.Exit
	httpGet = http.Get
	writeF  = os.WriteFile
)

func main() { osExit(run(os.Args[1:])) }

// run is the generator body. It returns a process exit code: 0 on success, 2 on
// a usage error, 1 on any fetch, parse or write failure.
func run(args []string) int {
	if len(args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: genindic <syllabic URL> <positional URL> <out.go>")
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
	src, err := generate(isc, ipc)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := writeF(args[2], src, 0o644); err != nil {
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

// iscToCat maps an Indic_Syllabic_Category value to the shaper's category
// constant name (defined in indic.go).
var iscToCat = map[string]string{
	"Avagraha":                    "catSymbol",
	"Bindu":                       "catSM",
	"Brahmi_Joining_Number":       "catPlaceholder",
	"Cantillation_Mark":           "catA",
	"Consonant":                   "catC",
	"Consonant_Dead":              "catC",
	"Consonant_Final":             "catCM",
	"Consonant_Head_Letter":       "catC",
	"Consonant_Initial_Postfixed": "catC",
	"Consonant_Killer":            "catX",
	"Consonant_Medial":            "catCM",
	"Consonant_Placeholder":       "catPlaceholder",
	"Consonant_Preceding_Repha":   "catRepha",
	"Consonant_Prefixed":          "catX",
	"Consonant_Subjoined":         "catCM",
	"Consonant_Succeeding_Repha":  "catRepha",
	"Consonant_With_Stacker":      "catCS",
	"Gemination_Mark":             "catSM",
	"Invisible_Stacker":           "catH",
	"Joiner":                      "catZWJ",
	"Modifying_Letter":            "catX",
	"Non_Joiner":                  "catZWNJ",
	"Nukta":                       "catN",
	"Number":                      "catPlaceholder",
	"Number_Joiner":               "catX",
	"Pure_Killer":                 "catH",
	"Register_Shifter":            "catRS",
	"Reordering_Killer":           "catH",
	"Syllable_Modifier":           "catSM",
	"Tone_Letter":                 "catA",
	"Tone_Mark":                   "catA",
	"Virama":                      "catH",
	"Visarga":                     "catSM",
	"Vowel":                       "catV",
	"Vowel_Dependent":             "catM",
	"Vowel_Independent":           "catV",
}

// ipcToPos maps an Indic_Positional_Category value to the shaper's raw position
// constant name. Left-side positions reorder before the base, right/below/top
// after it; the effective reorder position is refined per glyph role in indic.go.
var ipcToPos = map[string]string{
	"Bottom":                   "posBelowC",
	"Bottom_And_Left":          "posPreM",
	"Bottom_And_Right":         "posPostC",
	"Left":                     "posPreM",
	"Left_And_Right":           "posPreM",
	"Overstruck":               "posAboveC",
	"Right":                    "posPostC",
	"Top":                      "posAboveC",
	"Top_And_Bottom":           "posBelowC",
	"Top_And_Bottom_And_Left":  "posPreM",
	"Top_And_Bottom_And_Right": "posPostC",
	"Top_And_Left":             "posPreM",
	"Top_And_Left_And_Right":   "posPostC",
	"Top_And_Right":            "posPostC",
	"Visual_Order_Left":        "posPreM",
}

// generate builds the indictables.go source from the two property maps.
func generate(isc, ipc map[rune]string) ([]byte, error) {
	runes := map[rune]bool{}
	for r := range isc {
		runes[r] = true
	}
	for r := range ipc {
		runes[r] = true
	}
	sorted := make([]rune, 0, len(runes))
	for r := range runes {
		sorted = append(sorted, r)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	type entry struct {
		r        rune
		cat, pos string
	}
	entries := make([]entry, 0, len(sorted))
	for _, r := range sorted {
		cat := "catX"
		if v, ok := isc[r]; ok {
			c, ok := iscToCat[v]
			if !ok {
				return nil, fmt.Errorf("unmapped Indic_Syllabic_Category %q at U+%04X", v, r)
			}
			cat = c
		}
		pos := "posEnd"
		if v, ok := ipc[r]; ok {
			p, ok := ipcToPos[v]
			if !ok {
				return nil, fmt.Errorf("unmapped Indic_Positional_Category %q at U+%04X", v, r)
			}
			pos = p
		}
		entries = append(entries, entry{r, cat, pos})
	}

	var buf bytes.Buffer
	buf.WriteString("// Copyright (c) 2026 the go-opentype/shape authors. All rights reserved.\n")
	buf.WriteString("// Use of this source code is governed by a BSD-3-Clause license that can be\n")
	buf.WriteString("// found in the LICENSE file at the root of this repository.\n\n")
	buf.WriteString("// Code generated by cmd/genindic; DO NOT EDIT.\n\n")
	buf.WriteString("package shape\n\n")
	buf.WriteString("// indicData maps Unicode code point ranges to their Indic category and raw\n")
	buf.WriteString("// positional category, sorted by Lo. Code points in no range default to\n")
	buf.WriteString("// (catX, posEnd). Generated from the Unicode Character Database by cmd/genindic.\n")
	buf.WriteString("var indicData = []indicRange{\n")
	// Coalesce contiguous runes with identical (cat, pos) into ranges.
	i := 0
	for i < len(entries) {
		j := i + 1
		for j < len(entries) &&
			entries[j].r == entries[j-1].r+1 &&
			entries[j].cat == entries[i].cat &&
			entries[j].pos == entries[i].pos {
			j++
		}
		fmt.Fprintf(&buf, "\t{0x%04X, 0x%04X, %s, %s},\n", entries[i].r, entries[j-1].r, entries[i].cat, entries[i].pos)
		i = j
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}
