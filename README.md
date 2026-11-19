# Chord Diagram Studio

[![DOI](https://zenodo.org/badge/1428772029.svg)](https://doi.org/10.5281/zenodo.23602941)

Chord Diagram Studio turns Spearman correlation and p-value matrices from Excel into publication-ready chord diagrams. It shows only significant correlations between variable categories, and lets you exclude categories, choose which strength categories to draw, compare groups or time-points, combine several files (e.g. studies) in one diagram, add rings for category groups, regions and files, and count correlations by sign and strength.

**Use it online:** https://voulvet.github.io/ChordDiagramStudio/
Files are processed locally in your browser; nothing is uploaded.

**Desktop version** (Mac, Windows, Linux): see [Releases](../../releases).

Every option below works the same way on the website, in the desktop program and on the command line.

## Input

One Excel file with two sheets of the same size:

1. Spearman r matrix
2. p-value matrix

A full symmetric matrix or a single triangle both work. Row 1 and column A can contain either:

- **category names** (e.g. `Macroscopy`, `MicroCT`), one per variable, as in `example/example_matrices.xlsx`; or
- **variable names** (e.g. `MicroCT_BV/TV (MED)`), as in `example/example_variables_regions.xlsx`. The category is the text before the separator (default `_`). A **region** (or any other grouping) is read from a code in brackets at the end, `(MED)`, or a short upper-case code after the last separator, `_MED`. Alternatively, add a sheet called **Variables** with the columns `Variable`, `Category` and `Region`; the categories and regions are then taken exactly from that sheet (this is also the way to give categories longer display names).

Optional sheet **Categories** with the columns `Category`, `Label` and `Group`:

| Category | Label | Group |
|---|---|---|
| Cartilage histology | Histology | Articular cartilage |
| Cartilage biomechanics | Biomechanics | Articular cartilage |
| Subchondral bone microstructure | Microstructure | Subchondral bone |

- `Category`: the category name as the program sees it.
- `Label`: a short name shown inside the ring (optional; otherwise the category name).
- `Group`: a higher level drawn as an outer ring, e.g. the tissue (optional).
- The row order is the ring order (and so the colours), unless you change the order yourself.

The sheet is read from the data file (with several files, from the first file that has one). Labels and groups can also be typed into the category table. The example files `example/example_studyA_*.xlsx` contain both a Variables and a Categories sheet.

### Several Excel files

Choose **Several Excel files** and add the other files (same variables, or the same layout for different studies). Give every file a **group** and a **file label**:

- files with the **same group** are combined into one diagram: their correlations are drawn together, but a correlation is never computed across two files (e.g. two studies of the same condition);
- files with **different groups** are compared (e.g. Control and OA, or two time-points);
- the **file label** names each file in the diagram and the tables (e.g. Study A).

Layouts for several files:

- **Compare groups** (default when there are two or more groups): the **variable categories on the left** and the **groups on the right** (first group at the top). Each significant correlation is one chord from one of its two categories (left) to the group where it was found; the inner band of each group shows, in the category colours, the category at the other end of the correlation, explained by a legend under the diagram. Which end of a correlation is drawn on the left is balanced around the ring order, so that every category taking part in a correlation appears on the left (this only affects the drawing; counts and tables keep the ring order). **Highlight** one group to draw its chords in full colour and tone the others down (pale grey with a faint hint of the red/blue), for example to make one figure that emphasises the control group and one that emphasises the OA group; the counts are not affected, and the file names get `_highlight_<group>`.
- **One diagram per group** (full circle): one diagram per group with the correlations of all its files, e.g. one for Control and one for OA. Add rings for the files, regions or category groups (see Appearance).
- **One diagram per group, files on the right**: like the comparison, but the right half shows the **files** of the group (e.g. Study A, Study B), optionally divided into **regions** (Appearance → Extra rings → Region). Each chord ends at the file/region and category of the variable at its other end.

**Regions with the same code in different files** are kept **separate** by default (region AC of Study A is not region AC of Study B; the region is then called "Study A AC"), or can be **combined** (same code = same region).

All other options (exclude, regions, strength) apply to every file, and every count is given per group.

Example files: `example/example_timepoint_1.xlsx` and `example/example_timepoint_2.xlsx` (one study, two time-points), and `example/example_studyA_control.xlsx`, `example_studyA_oa.xlsx`, `example_studyB_control.xlsx`, `example_studyB_oa.xlsx` (two studies × two groups). All example data are synthetic (made with `tools/make_examples.py`); they do not come from any study.

## What the diagram shows

- one chord per correlation with p ≤ the chosen threshold (default 0.05)
- no correlations within the same category
- chord colour from soft blue (r = −1) through white (0) to soft red (+1)
- equal-size pastel ring segments (proportional segments only when you ask for them, see Appearance); ribbons that do not weave

## Options

- **Exclude** categories
- **Ring order** and fixed colours per category
- **One diagram per region**: only correlations between variables of the same region; variables without a region (e.g. whole-joint scores) can be included in every region or left out. Their name in rings and tables can be set (default "No region").
- **Only correlations within the same region** in a single diagram
- **Several files**: compare groups, combine files, layouts and regions as described above; optionally **highlight** one group
- **Strength categories** (|r|, positive or negative): very strong ≥ 0.80, strong 0.60 – < 0.80, moderate 0.40 – < 0.60, weak < 0.40 (fixed cut-offs). Tick the categories to include in the diagram (default: all). The diagram, its counts and its tables then contain only the ticked categories, and the title says which ones are shown (e.g. "strong and very strong correlations, |r| ≥ 0.60"). The changes workbook always lists every significant change, with its strength category, so it can be filtered there.
- Titles and file names are made automatically from the Excel file name and the settings.

### Appearance

- **Chord thickness**
  - *Automatic* (default): the chords of a category fill its ring segment, so their thickness depends on the number of correlations.
  - *Same in every diagram of this run*: every chord has the same width in every diagram that is made together (one per region, several groups). The program uses the widest chord that fits the busiest diagram; where there is more room, chords keep this width and are spaced further apart. The width used is shown with the results and in the summary workbook.
  - *Fixed*: a width in degrees (e.g. 0.2). Ring segments stay equal unless a category needs more room; then that segment grows and the others share the rest. If the chords do not fit, the program says which width would fit.
- **Label size** of the category names (pt) and **characters per line**. Automatic: 12 pt with 30 characters per line in the full circle, 21 pt with 20 characters in the comparison layouts. Leader lines always stay outside the ring: a label that had to move away from its segment gets a leader that goes out horizontally, then vertically, and leaders never cross.
- **Count scale on the rings**: a scale on every segment counting its chords from 0 at its clockwise start, with a tick every 5 chords and a number every 25 (every 5 in segments with fewer than 25 chords). With the scale, segments are proportional to the number of chords.
- **Segment sizes**: automatic (proportional with the scale, else equal), equal, or proportional to the number of chords.
- **Extra rings** (full circle: from the inside out, category > group > region > file; files on the right: region). *Group* needs the Group column of the Categories sheet. Gaps are larger between groups, regions and files than between categories.
- **Names of ring segments**: *inside* the rings (letters placed one by one along the ring, upright: clockwise on the top half, anticlockwise on the bottom half; names shrink to fit, and segments are at least 24° wide when only groups are added so every name fits), or *outside* with leader lines (no text in the rings; category names in a legend under the diagram). Automatic = inside when there are extra rings. Category names that do not fit in their segment are listed in a legend.
- **What the Group column means** (e.g. "tissue") and **what the files are** (e.g. "study"): used in the note under the diagram ("Rings from inside out: number of chords, variable category, tissue").
- Colours of the group and file rings are chosen automatically so that they cannot be confused with the category colours (for example, never a second shade of a category's green or red); with many categories they are neutral greys.

## Outputs

PNG (300 dpi), SVG (vector) and an Excel file with counts in total, by sign, by strength, per category pair, per category, and a list of every correlation drawn (variable names, categories, r, p, sign, strength, and the regions and file of the two variables). With the scale or extra rings, a sheet lists the chord ends per ring segment. Per-region runs and runs with several groups also produce a summary workbook comparing all diagrams. In a comparison, every sheet has a column for the group.

### Changes between two groups

When the files belong to **two groups**, a workbook `<prefix>_changes.xlsx` lists every correlation between different categories (excluded categories left out) that

- **appeared**: significant only in the second group,
- **disappeared**: significant only in the first group, or
- **changed sign**: significant in both groups with opposite sign (positive → negative or negative → positive),

sorted by strength (|r| of the significant correlation; for a sign change the larger |r|), with r and p in both groups, Δr, the strength band and the region (the region code; "No region (all regions)" when both variables have no region; "Across regions (A–B)" when the two variables belong to different regions). Sheets: Summary (counts per type, per region, the unchanged correlations, and a check that appeared + sign change + unchanged = the second group's count and disappeared + sign change + unchanged = the first group's count, for every region); All changes; Appeared; Disappeared; Sign change; one sheet per region (variables without a region count in every region, as in the per-region diagrams). Variables are matched by name; case, spaces and underscores do not matter. With several files per group, variables are matched within files that have the same file label.

Note: "disappeared" only means that the correlation is no longer significant; r may still be large, in either direction.

### Checks on the input files

- A damaged sheet name (longer than 31 characters or with characters Excel does not allow) no longer stops the program: the sheet is read by its position and the new name is reported.
- Non-breaking spaces and repeated spaces in names are treated as normal single spaces.
- Names that differ only in spacing, underscores or case (e.g. `Lining thickness` / `Lining_thickness`) are matched as one variable and listed.
- Words in variable names that look like typos of each other (one letter different or two letters swapped, e.g. `Proteoglycan` / `Proteoglycna`) are listed so you can check them.

These notes are shown after loading the files and with the results, printed by the command line, and written to a "Warnings" sheet of the counts workbooks.

## Command line

The desktop program also runs from a terminal, for example:

```
./chordstudio -exclude "Immunohistochemistry,Biochemistry" -bygroup all data.xlsx
./chordstudio -strength "very strong,strong" data.xlsx
./chordstudio -variables -withingroup data.xlsx
./chordstudio -compare oa.xlsx -timenames "Control,OA" -bygroup all control.xlsx
./chordstudio -compare oa.xlsx -timenames "Control,OA" -highlight OA control.xlsx
./chordstudio -chordwidth same -labelsize 21 -bygroup all data.xlsx
./chordstudio -usesheet -scale -rings group -groupword tissue data.xlsx
./chordstudio -out results -bygroup all data.xlsx
```

Output files are written next to the first Excel file, or into the folder given with `-out`; their names are made from the Excel file name.

Several files (flags before the file names):

```
# two studies combined, one full-circle diagram per group, with tissue and count rings
./chordstudio -usesheet -groupnames "Control,Control,OA,OA" -filelabels "Study A,Study B,Study A,Study B" \
    -layout ring -scale -rings group -groupword tissue -chordwidth same \
    studyA_control.xlsx studyB_control.xlsx studyA_oa.xlsx studyB_oa.xlsx

# one diagram per group: categories left, studies and regions right
./chordstudio -usesheet -groupnames "Control,Control,OA,OA" -filelabels "Study A,Study B,Study A,Study B" \
    -layout split -scale -rings region -chordwidth same -fileword study \
    studyA_control.xlsx studyB_control.xlsx studyA_oa.xlsx studyB_oa.xlsx
```

Options added in version 2.0 and later:

| Option | Meaning |
|---|---|
| `-groupnames "A,A,B,B"` | group of each file (same group = combined; different groups = compared). `-timenames` and `-compare` still work for two files |
| `-filelabels "S1,S2,S1,S2"` | label of each file (default: the file name) |
| `-layout compare\|ring\|split` | several files: compare groups / one full circle per group / one per group with the files on the right |
| `-regions separate\|combined` | regions with the same code in different files |
| `-highlight 1\|name` | comparison: group shown in full colour |
| `-noregion "No region"` | name for variables without a region |
| `-chordwidth auto\|same\|0.2` | chord thickness (degrees for a fixed width) |
| `-labelsize 21`, `-labelwrap 20` | category labels (0 = automatic) |
| `-scale` | count scale on the rings |
| `-segments auto\|equal\|proportional` | ring segment sizes |
| `-rings group,region,file` | extra rings |
| `-ringlabels auto\|inside\|outside` | names inside the rings or outside with leader lines |
| `-strength "very strong,strong"` | strength categories to draw (default: all); version 3.0 |
| `-out folder` | folder for the output files (version 3.0: file names are automatic) |
| `-groupword tissue`, `-fileword study` | what the groups and files are (legend text) |

Run `./chordstudio -h` for all options.

## Building

Requires Go 1.24 or newer. Run `./build.sh`. It writes the website to `docs/` and the desktop programs to `dist/`.

- `core.go`: reading data, filtering, counting, classic drawing
- `vars.go`: variable names, categories and regions
- `hygiene.go`: damaged sheet names, name normalisation and checks
- `draw.go`: appearance options, segment sizes, scale, curved and outside labels, ring colours
- `rings.go`: full-circle layout with extra rings
- `comparelayout.go`, `compare.go`: comparison layouts (groups or files on the right) and their counts
- `changes.go`: changes between two groups
- `gen.go`, `session.go`: single diagrams, regions, several files
- `server.go`, `main.go`, `ui.html`: desktop app (local browser interface) and command line
- `wasm_main.go`, `docs/index.html`: website build (WebAssembly); `tools/mkweb.py` creates `docs/index.html` from `ui.html`
- `tools/make_examples.py`: creates the synthetic example files
- `tools/package_release.py`: builds the desktop zips from `dist/`

Releases: on GitHub, Actions → Release → "Run workflow" (enter the version, e.g. `3.1.0`, and tick "Draft only" for a test), or push a version tag (e.g. `v3.1.0`). The workflow (`.github/workflows/release.yml`) checks the version in `CITATION.cff`, builds the website and the desktop programs, packages the zips and publishes the GitHub release with the text from `release-notes/v<version>.md`. Tags with a hyphen (e.g. `v3.1.0-test`) give a draft release.

Removed in version 3.0: combine categories (`-merge`), keep apart (`-apart`), one diagram per category (`-series`), own titles (`-title`), own file-name prefix, and reading the Categories sheet from a separate file (`-categories`).

## How to cite

If you use Chord Diagram Studio in a publication or presentation, please cite:

> Carretero-Hernandez M. Chord Diagram Studio (version 3.0.0) [Software]. Zenodo. https://doi.org/10.5281/zenodo.24362219

## AI disclosure

The source code of Chord Diagram Studio was generated with Claude (Anthropic), an AI assistant, based on the author's specifications. The author defined the requirements and design, and tested and validated the program's output against independent calculations. The program draws figures directly from the input data; it does not use AI to create or modify them. The example data are synthetic and were made with a script (`tools/make_examples.py`) that was also generated with Claude.
