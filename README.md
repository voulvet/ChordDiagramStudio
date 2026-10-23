[![DOI](https://zenodo.org/badge/1428772029.svg)](https://doi.org/10.5281/zenodo.23602941)

# Chord Diagram Studio

Chord Diagram Studio turns Spearman correlation and p-value matrices from Excel into publication-ready chord diagrams. It shows only significant correlations between variable categories, and lets you exclude, combine or keep apart categories, build a series of diagrams, and count correlations by sign and strength.

**Use it online:** https://voulvet.github.io/ChordDiagramStudio/

Files are processed locally in your browser; nothing is uploaded.

**Desktop version** (Mac, Windows, Linux): see [Releases](../../releases).

## Input

One Excel file with two sheets of the same size:

1. Spearman r matrix
2. p-value matrix

Row 1 and column A contain the **category** of each variable. A full symmetric matrix or a single triangle both work. See `example/example_matrices.xlsx`.

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
- **Series**: one diagram per selected category
- **Strength categories** (|r|): very strong ≥ 0.80, strong ≥ 0.60, moderate ≥ 0.40, weak below (adjustable)

Outputs: PNG (300 dpi), SVG (vector) and an Excel file with counts in total, by sign, by strength, per category pair and per category.

## Building

Requires Go 1.24 or newer. Run `./build.sh`. It writes the website to `docs/` and the desktop programs to `dist/`.

- `core.go`: reading data, filtering, counting, drawing
- `gen.go`: single diagrams and series
- `server.go`, `main.go`, `ui.html`: desktop app (local browser interface) and command line
- `wasm_main.go`, `docs/index.html`: website build (WebAssembly)

## AI disclosure

The source code of Chord Diagram Studio was generated with Claude (Anthropic), an AI assistant, based on the author's specifications. The author defined the requirements and design, and tested and validated the program's output against independent calculations. The program draws figures directly from the input data; it does not use AI to create or modify them.
