# Chord Diagram Studio

[![DOI](https://zenodo.org/badge/1428772029.svg)](https://doi.org/10.5281/zenodo.23602941)

Chord Diagram Studio turns Spearman correlation and p-value matrices from Excel into publication-ready chord diagrams. It shows only significant correlations between variable categories, and lets you exclude, combine or keep apart categories, build a series of diagrams, and count correlations by sign and strength.

**Use it online:** https://voulvet.github.io/ChordDiagramStudio/
Files are processed locally in your browser; nothing is uploaded.

**Desktop version** (Mac, Windows, Linux): see [Releases](../../releases).

## Input

One Excel file with two sheets of the same size:

1. Spearman r matrix
2. p-value matrix

A full symmetric matrix or a single triangle both work. Row 1 and column A can contain either:

- **category names** (e.g. `Macroscopy`, `MicroCT`), one per variable, as in `example/example_matrices.xlsx`; or
- **variable names** (e.g. `MicroCT_SCBP BMD (AE)`), as in `example/example_variables_regions.xlsx`. The category is the text before the separator (default `_`). A **region** (or any other grouping) is read from a code in brackets at the end, `(AE)`, or a short upper-case code after the last separator, `_AE`. Alternatively, add a sheet called **Variables** with the columns `Variable`, `Category` and `Region`.

## What the diagram shows

- one chord per correlation with p ≤ the chosen threshold (default 0.05)
- no correlations within the same category
- chord colour from soft blue (r = −1) through white (0) to soft red (+1)
- equal-size ring segments in pastel colours; ribbons that do not weave

## Options

- **Exclude** categories
- **Combine** all categories containing a keyword into one category
- **Keep apart**: no comparisons between categories sharing a keyword (e.g. "day")
- **Ring order** and fixed colours per category
- **One diagram per category** (series)
- **One diagram per region**: only correlations between variables of the same region; variables without a region (e.g. whole-joint scores) can be included in every region or left out
- **Only correlations within the same region** in a single diagram
- **Strength categories** (|r|): very strong ≥ 0.80, strong ≥ 0.60, moderate ≥ 0.40, weak below (adjustable)

Outputs: PNG (300 dpi), SVG (vector) and an Excel file with counts in total, by sign, by strength, per category pair, per category, and a list of every correlation drawn (variable names, r, p, sign, strength). Series and per-region runs also produce a summary workbook comparing all diagrams.

## Command line

The desktop program also runs from a terminal, for example:

```
./chordstudio -exclude "Immunohistochemistry,Biochemistry" -bygroup all data.xlsx
./chordstudio -apart day data.xlsx
./chordstudio -variables -withingroup data.xlsx
```

Run `./chordstudio -h` for all options.

## Building

Requires Go 1.24 or newer. Run `./build.sh`. It writes the website to `docs/` and the desktop programs to `dist/`.

- `core.go`: reading data, filtering, counting, drawing
- `vars.go`: variable names, categories and regions
- `gen.go`: single diagrams and series
- `server.go`, `main.go`, `ui.html`: desktop app (local browser interface) and command line
- `wasm_main.go`, `docs/index.html`: website build (WebAssembly); `tools/mkweb.py` creates `docs/index.html` from `ui.html`

## How to cite

If you use Chord Diagram Studio in a publication or presentation, please cite:

> Carretero-Hernandez M. Chord Diagram Studio (version 1.1.0) [Software]. Zenodo. https://doi.org/10.5281/zenodo.23781656

## AI disclosure

The source code of Chord Diagram Studio was generated with Claude (Anthropic), an AI assistant, based on the author's specifications. The author defined the requirements and design, and tested and validated the program's output against independent calculations. The program draws figures directly from the input data; it does not use AI to create or modify them.
