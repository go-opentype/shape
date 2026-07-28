# go-opentype/shape

[![CI](https://github.com/go-opentype/shape/actions/workflows/ci.yml/badge.svg)](https://github.com/go-opentype/shape/actions/workflows/ci.yml)
[![pkg.go.dev](https://img.shields.io/badge/pkg.go.dev-shape-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/go-opentype/shape)
![coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)
![go](https://img.shields.io/badge/Go-1.26.4%2B-00ADD8?logo=go&logoColor=white)
[![license](https://img.shields.io/badge/license-BSD--3--Clause-blue)](./LICENSE)

A pure-Go, **CGO=0, standard-library-only** complex-text shaper — a HarfBuzz-lite
for the [go-opentype](https://github.com/go-opentype) stack. It turns a run of
Unicode text into positioned glyphs in **visual order**, so real Arabic (and
Latin) renders correctly instead of as isolated, unattached glyphs.

It composes two siblings and adds nothing else: bidi reordering from
[`go-opentype/bidi`](https://github.com/go-opentype/bidi) and GSUB/GPOS from
[`go-opentype/opentype`](https://github.com/go-opentype/opentype). No
`golang.org/x/*`, no third-party modules; it builds for every Go target
including `GOOS=js GOARCH=wasm`.

## What it does

- **Bidirectional reordering** — resolves UAX #9 embedding levels and lays the
  glyphs out left-to-right, so a right-to-left Arabic run is emitted in drawing
  order.
- **Arabic cursive joining** — resolves each letter's joining form (isolated /
  initial / medial / final) and applies the font's `isol`/`init`/`medi`/`fina`
  GSUB features **positionally**, each only at the glyphs in that form. Joining
  forms are tracked through `ccmp` decomposition (the rasm-skeleton-plus-dots
  architecture real fonts such as Noto Sans Arabic use), so the joined glyphs
  actually connect.
- **Ligatures, mark attachment, kerning** — GSUB `ccmp`/`rlig`/`liga`/`calt`
  then GPOS `kern`/`mark`/`mkmk`/`curs`, so diacritics sit on their base and
  pairs kern.

## Usage

```go
import (
    "github.com/go-opentype/opentype"
    "github.com/go-opentype/shape"
)

f, _ := opentype.Parse(ttf) // ttf is a []byte TrueType/OpenType blob
face := f.NewFace(32)        // 32px per em

for _, g := range shape.Shape(face, "بيت", shape.Options{}) {
    // g.GID      glyph to draw
    // g.Cluster  source rune index it derives from
    // g.XOffset, g.YOffset   placement relative to the pen (px)
    // g.XAdvance, g.YAdvance advance to move the pen by (px)
}
```

The base direction defaults to `Auto` (from the first strong character) and the
script is auto-detected (any Arabic-block rune selects the Arabic shaper) unless
you set `Options.Script` (`"arab"`, `"latn"`, `"dflt"`) or `Options.Direction`.

## API

```go
type Glyph struct {
    GID      opentype.GlyphIndex
    Cluster  int
    XAdvance int
    YAdvance int
    XOffset  int
    YOffset  int
}

type Options struct {
    Direction bidi.Direction // LeftToRight, RightToLeft, Auto
    Script    string         // "arab", "latn", "dflt"; empty auto-detects
    Features  []string       // extra feature tags
}

func Shape(face *opentype.Face, text string, opts Options) []Glyph
```

## Scope

Implemented: **Arabic** and **Latin/default** (Latin, Cyrillic, Greek, CJK, …)
shaping — cursive joining, ligatures, mark attachment, kerning, bidi visual
order.

Out of scope (future work): scripts that need glyph reordering or a state
machine — **Indic** (Devanagari, …), **Thai/Lao**, **Khmer**, **Myanmar** and
the **Universal Shaping Engine**. They currently shape through the default path
(no reordering). Cluster indices are exact for one-to-one substitutions (the
Arabic positional forms) and best-effort, monotonic, when a substitution changes
the run length (ligatures, decomposition).

## License

BSD-3-Clause. See [LICENSE](./LICENSE).
