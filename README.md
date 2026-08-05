# go-opentype/shape

[![CI](https://github.com/go-opentype/shape/actions/workflows/ci.yml/badge.svg)](https://github.com/go-opentype/shape/actions/workflows/ci.yml)
[![pkg.go.dev](https://img.shields.io/badge/pkg.go.dev-shape-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/go-opentype/shape)
![coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)
![go](https://img.shields.io/badge/Go-1.26.4%2B-00ADD8?logo=go&logoColor=white)
[![license](https://img.shields.io/badge/license-BSD--3--Clause-blue)](./LICENSE)

A pure-Go, **CGO=0, standard-library-only** complex-text shaper — a HarfBuzz-lite
for the [go-opentype](https://github.com/go-opentype) stack. It turns a run of
Unicode text into positioned glyphs in **visual order**, so Arabic, Indic,
Southeast-Asian, CJK vertical and Egyptian Hieroglyph text renders correctly
instead of as isolated, unattached, unreordered glyphs.

It composes two siblings and adds nothing else: bidi reordering from
[`go-opentype/bidi`](https://github.com/go-opentype/bidi) and GSUB/GPOS from
[`go-opentype/opentype`](https://github.com/go-opentype/opentype). No
`golang.org/x/*`, no third-party modules; it builds for every Go target
including `GOOS=js GOARCH=wasm`.

## Install

```sh
go get github.com/go-opentype/shape
```

## Quick start

```go
package main

import (
	"fmt"

	"github.com/go-opentype/fonts/notosansarabic"
	"github.com/go-opentype/opentype"
	"github.com/go-opentype/shape"
)

func main() {
	f, err := opentype.Parse(notosansarabic.TTF)
	if err != nil {
		panic(err)
	}
	face := f.NewFace(32) // 32px per em

	for _, g := range shape.Shape(face, "بيت", shape.Options{}) {
		// g.GID              glyph to draw
		// g.Cluster          source rune index it derives from
		// g.XOffset, g.YOffset   placement relative to the pen (px)
		// g.XAdvance, g.YAdvance advance to move the pen by (px)
		// g.Scale            draw scale (< 1 inside an Egyptian quadrat)
		fmt.Println(g)
	}
}
```

The base direction defaults to `Auto` (from the first strong character) and
the script is auto-detected from the text (any Arabic-block rune selects
Arabic, Indic/USE runes select their shaper, ...) unless you set
`Options.Script` or `Options.Direction`.

## Features

- **Bidirectional reordering** — resolves UAX #9 embedding levels (via
  `go-opentype/bidi`) and lays glyphs out left-to-right, so a right-to-left
  run is emitted in drawing order.
- **Arabic cursive joining** — resolves each letter's joining form (isolated
  / initial / medial / final) and applies the font's
  `isol`/`init`/`medi`/`fina` GSUB features positionally, each only at the
  glyphs in that form, so letters actually connect (including fonts built on
  the rasm-skeleton-plus-dots architecture, such as Noto Sans Arabic).
- **Indic shaping** — Devanagari, Bengali, Gurmukhi, Gujarati, Oriya, Tamil,
  Telugu, Kannada, Malayalam and Sinhala, via the HarfBuzz "indic" model:
  syllable splitting, base/reph detection, two reordering passes (pre-base
  matras and reph), the full basic + presentation GSUB feature pipeline, and
  GPOS mark/mkmk/abvm/blwm attachment.
- **Universal Shaping Engine (USE)** — the general complex-script model for
  the scripts without a bespoke shaper: Thai, Lao, Khmer, Myanmar, Tibetan,
  Javanese, Balinese, Buginese, Tai Tham and more. Runs are classified into
  USE syllabic categories, split into clusters, reordered (pre-base vowels
  and modifiers before the base, repha after it) and run through the USE
  GSUB/GPOS pipeline, with sakot/halant joining, split-vowel decomposition
  and dotted-circle insertion for defective clusters.
- **Egyptian Hieroglyph quadrats** — the Unicode format-control characters
  U+13430–U+1345F (joiners, corner insertions, overlays, segment/enclosure
  delimiters) are parsed into a two-dimensional quadrat tree and laid out
  geometrically, so a run of signs renders as compact blocks.
- **Hangul** — conjoining jamo (leading/vowel/trailing) are composed into
  precomposed syllable blocks before shaping, so decomposed Korean input
  renders as single Hangul glyphs.
- **Vertical writing mode (CJK tategaki)** — `Options.Vertical` selects the
  `vert`/`vrt2` upright glyph forms and stacks glyphs top-to-bottom using the
  font's vertical metrics (`vmtx`/`VORG`).
- **Ligatures, mark attachment, kerning** — GSUB `ccmp`/`rlig`/`liga`/`calt`
  then GPOS `kern`/`mark`/`mkmk`/`curs` for every script path, so diacritics
  sit on their base and pairs kern.

## API tour

```go
type Glyph struct {
    GID      opentype.GlyphIndex // glyph to draw
    Cluster  int                 // source rune index
    XAdvance int
    YAdvance int
    XOffset  int
    YOffset  int
    Scale    float64 // 1.0 normally; < 1 inside an Egyptian quadrat
}

type Options struct {
    Direction bidi.Direction // LeftToRight, RightToLeft, Auto (default)
    Script    string         // "arab", "latn", "dflt", a script tag, or "" to auto-detect
    Features  []string       // extra feature tags applied over the whole run
    Vertical  bool           // CJK tategaki: top-to-bottom, vert/vrt2 forms
}

func Shape(face *opentype.Face, text string, opts Options) []Glyph

// DeriveUSECategory maps Unicode Indic_Syllabic_Category / Indic_Positional_Category /
// General_Category to a USE syllabic category; exported for cmd/genuse.
func DeriveUSECategory(uisc, uipc, ugc string) string
```

See [`example_test.go`](./example_test.go) for runnable examples (an Arabic
word with cursive joining, plain Latin, and forcing `Options`), and
`go doc github.com/go-opentype/shape` for the full reference, including the
Indic, USE, Egyptian Hieroglyph and Vertical sections.

## Scope

Implemented: **Arabic**, the ten dedicated-shaper **Indic** scripts, the
**Universal Shaping Engine** (Thai, Lao, Khmer, Myanmar, Tibetan, Javanese,
Balinese, Buginese, Tai Tham, ...), **Egyptian Hieroglyph** quadrat layout,
**Hangul** jamo composition, **vertical** writing mode, and **Latin/default**
(Latin, Cyrillic, Greek, CJK horizontal, ...) shaping.

Cluster indices are exact for one-to-one substitutions (the Arabic
positional forms) and best-effort, monotonic, when a substitution changes
the run length (ligatures, decomposition, Indic/USE reordering).

## Testing

Most branches are exercised with synthetic in-memory fonts; a handful of
real-font sanity checks run against bundled `go-opentype/fonts` families
(Noto Sans Arabic for cursive joining, and Noto Sans Balinese/Khmer/Myanmar/
Tai Tham under [`testdata`](./testdata) for USE) to confirm the shaper
actually joins, reorders and positions glyphs in production fonts, not just
the synthetic test doubles. CI enforces **100.0% statement coverage**,
`go vet`, and cross-compilation for the six 64-bit architectures plus
`js/wasm`, `darwin/arm64` and `windows/amd64`.

## Part of the go-opentype pure-Go text stack

`go-opentype/shape` is the HarfBuzz-lite shaping layer of a dependency-free
text stack:

- **[opentype](https://github.com/go-opentype/opentype)** — the parsing,
  GSUB/GPOS shaping-primitives and rasterising engine this package builds
  its `Shape` call on.
- **[bidi](https://github.com/go-opentype/bidi)** — the Unicode
  Bidirectional Algorithm (UBA) implementation that resolves this package's
  base direction and reordering.
- **[shape](https://github.com/go-opentype/shape)** (this repo) — the
  complex-script shaper.
- **[fonts](https://github.com/go-opentype/fonts)** — 46 bundled OFL/BSD
  font families (Latin, non-Latin scripts and CJK), per-family lazily
  `go:embed`-ed, used throughout this repo's real-font tests and examples.

## License

BSD-3-Clause. See [LICENSE](./LICENSE).
