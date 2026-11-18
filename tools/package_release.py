"""Builds the desktop zips (one per platform) from dist/ after ./build.sh.

Usage: python3 tools/package_release.py VERSION [OUTDIR]
Each zip contains the program, README.txt, the example files and the Go source code
(folder "source"); the Mac zips also contain "Chord Diagram Studio.app".
"""
import os, sys, zipfile, stat

VERSION = sys.argv[1]
OUT = sys.argv[2] if len(sys.argv) > 2 else 'release'
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
os.makedirs(OUT, exist_ok=True)

MAC_FIRST = '''FIRST TIME ONLY
  macOS blocks apps downloaded from outside the App Store. After unzipping,
  open Terminal and run (adjust the path if you moved the folder):

      xattr -cr ~/Downloads/{folder}

  Then double-click "Chord Diagram Studio.app".
  (Optional: drag the app into your Applications folder.)
'''
LINUX_FIRST = '''FIRST TIME ONLY
  After unzipping, make the program executable and start it:

      chmod +x chordstudio
      ./chordstudio

  Your default browser opens the interface (if it does not, open the
  address printed in the terminal).
'''
WIN_FIRST = '''FIRST TIME ONLY
  Unzip the folder first (right-click the zip > Extract All), then
  double-click ChordDiagramStudio.exe.
  Windows may show "Windows protected your PC" because the program is not
  signed by a publisher: click "More info" > "Run anyway".
  If the firewall asks, you can click Cancel: the program only talks to your
  own computer.
  A black console window opens and your browser shows the interface.
  Keep the black window open while you work; closing it stops the program.
'''

PLATFORMS = [
    # folder, title, binary in dist/, program name in the zip, first-time text, quit text, command intro, mac
    ('ChordDiagramStudio-mac-apple', 'Mac, Apple chip', 'chordstudio-mac-apple', 'chordstudio', MAC_FIRST,
     'Press "Quit" in the page when you are done.', 'The file "chordstudio" is the same program for Terminal use:', True),
    ('ChordDiagramStudio-mac-intel', 'Mac, Intel processor', 'chordstudio-mac-intel', 'chordstudio', MAC_FIRST,
     'Press "Quit" in the page when you are done.', 'The file "chordstudio" is the same program for Terminal use:', True),
    ('ChordDiagramStudio-windows', 'Windows 10/11, 64-bit', 'ChordDiagramStudio.exe', 'ChordDiagramStudio.exe', WIN_FIRST,
     'Press "Quit" in the page (or close the black window) when you are done.', '"ChordDiagramStudio.exe" can also be used from a terminal:', False),
    ('ChordDiagramStudio-linux-x64', 'Linux, x86-64', 'chordstudio-linux-x64', 'chordstudio', LINUX_FIRST,
     'Press "Quit" in the page when you are done.', '"chordstudio" can also be used from a terminal:', False),
    ('ChordDiagramStudio-linux-arm64', 'Linux, ARM64 e.g. Raspberry Pi 4/5', 'chordstudio-linux-arm64', 'chordstudio', LINUX_FIRST,
     'Press "Quit" in the page when you are done.', '"chordstudio" can also be used from a terminal:', False),
]

BODY = '''USING IT
  The app opens a page in your web browser. Everything runs on your own
  computer; nothing is uploaded to the internet.

  Data          Choose the Excel file. Sheet 1 = Spearman r matrix,
                sheet 2 = p-value matrix. Set the p threshold (0.05).
                Row 1 and column A contain either CATEGORY names
                (e.g. MicroCT) or VARIABLE names (e.g. MicroCT_BV/TV (MED)).
                The program suggests which one; you can change it.
                With variable names, the category is the text before "_"
                and a region is read from "(MED)" or "_MED" at the end of the
                name. A sheet called "Variables" (columns Variable,
                Category, Region) can be used instead. An optional sheet
                "Categories" (columns Category, Label, Group) gives short
                labels for the rings and a group (e.g. tissue) per category;
                its row order is the ring order.
  Input         "One Excel file" or "Several Excel files". With several
                files, give every file a group and a file label: files of
                the same group are combined (e.g. two studies), different
                groups are compared (e.g. Control and OA). Layouts:
                "Compare groups" (categories left, groups right; optional
                "Highlight"), "One diagram per group" (full circle), or
                "One diagram per group: categories left, files right".
                Regions with the same code in different files are kept
                separate or combined.
  Regions       (variable names only) Rename the grouping, e.g. "Region".
                Variables without a region (e.g. whole-joint scores) can be
                included in every region or left out, and get a name.
  Combine       "Combine categories": every category containing a keyword
                (e.g. "day") becomes one category with the name you give.
                "Keep apart": categories containing the keyword are not
                compared with each other.
  Categories    Untick a category to exclude it. Arrows change the order
                around the ring. Each category keeps its colour. Label and
                Group can be typed in or read from a "Categories" sheet.
  Diagrams      "One diagram", "One per category", or "One per region"
                (only correlations between variables of the same region).
                In "One diagram" you can also keep only correlations within
                the same region.
  Appearance    Chord thickness: automatic, the same in every diagram of
                the run (widest that fits), or a fixed width in degrees.
                Label size and characters per line. Count scale on the
                rings. Segment sizes (equal or proportional). Extra rings
                for the category group, region and file. Names inside the
                rings (curved) or outside with leader lines.
  Strength      Cut-offs for very strong / strong / moderate (|r|, positive
                or negative). Default 0.80 / 0.60 / 0.40; below = weak.

  Press Generate. For every diagram you see the figure plus counts:
  total, positive, negative, and per strength category; per category pair
  and per category tables are under "Counts per ...".
  Download PNG (300 dpi), SVG (vector) and an Excel file with all counts
  and a list of every correlation drawn. Series, per-region runs and runs
  with several groups also give a Summary workbook comparing the diagrams.
  With two groups, "Changes (xlsx)" lists the correlations that appeared,
  disappeared or changed sign, sorted by strength, with one sheet per
  region. "Download all (zip)" saves everything.

  {quit}

ALWAYS APPLIED
  - only correlations with p <= threshold
  - no correlations within the same category
  - one thin chord per correlation, soft blue (-1) / white (0) / soft red (+1)
  - equal-size pastel ring segments (proportional only if you choose it),
    smooth ribbons that do not weave
  - notes on the input files: damaged sheet names, names that differ only
    in spaces or underscores, and possible typos

COMMAND LINE (optional)
  {cmdintro}
      {p}                      opens the interface
      {p} -apart day data.xlsx
      {p} -merge "day=Pain scoring" data.xlsx
      {p} -series "Day 1 ...,Day 2 ..." -apart day data.xlsx
      {p} -exclude "Biochemistry" -alpha 0.01 data.xlsx
      {p} -bygroup all -exclude "Biochemistry" data.xlsx
      {p} -compare oa.xlsx -timenames "Control,OA" control.xlsx
      {p} -chordwidth same -labelsize 21 -bygroup all data.xlsx
      {p} -usesheet -groupnames "Control,Control,OA,OA"
          -filelabels "Study A,Study B,Study A,Study B" -layout ring
          -scale -rings group -groupword tissue -chordwidth same
          a_control.xlsx b_control.xlsx a_oa.xlsx b_oa.xlsx
      (options go before the file names; run with -h to see all options)

  Examples (synthetic data): example_matrices.xlsx (category names),
  example_variables_regions.xlsx (variable names with regions),
  example_timepoint_1.xlsx + example_timepoint_2.xlsx (comparison), and
  example_studyA/B_control/oa.xlsx (two studies x two groups, with
  Variables and Categories sheets).
  The "source" folder contains the Go source code.
'''

PLIST = '''<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>Chord Diagram Studio</string>
  <key>CFBundleDisplayName</key><string>Chord Diagram Studio</string>
  <key>CFBundleIdentifier</key><string>local.marta.chorddiagramstudio</string>
  <key>CFBundleVersion</key><string>{v}</string>
  <key>CFBundleShortVersionString</key><string>{v}</string>
  <key>CFBundleExecutable</key><string>chordstudio</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
</dict>
</plist>
'''

SOURCE = ['build.sh', 'go.mod', 'go.sum', 'core.go', 'vars.go', 'hygiene.go', 'draw.go', 'rings.go', 'comparelayout.go',
          'compare.go', 'changes.go', 'gen.go', 'session.go', 'main.go', 'server.go', 'wasm_main.go', 'ui.html',
          'tools/mkweb.py', 'tools/make_examples.py', 'tools/package_release.py']
EXAMPLES = sorted(f for f in os.listdir(os.path.join(ROOT, 'example')) if f.endswith('.xlsx'))


def add(z, name, data, exe=False):
    info = zipfile.ZipInfo(name, date_time=(2026, 1, 1, 0, 0, 0))
    info.compress_type = zipfile.ZIP_DEFLATED
    info.external_attr = ((0o755 if exe else 0o644) | stat.S_IFREG) << 16
    z.writestr(info, data)


for folder, title, binary, prog, first, quit_, cmdintro, mac in PLATFORMS:
    head = f'CHORD DIAGRAM STUDIO {VERSION}  ({title})'
    readme = head + '\n' + '=' * len(head) + '\n\n' + first.format(folder=folder) + '\n'
    p = prog if prog.endswith('.exe') else './' + prog
    readme += BODY.replace('{quit}', quit_).replace('{cmdintro}', cmdintro).replace('{p}', p)
    exe = open(os.path.join(ROOT, 'dist', binary), 'rb').read()
    path = os.path.join(OUT, folder + '.zip')
    with zipfile.ZipFile(path, 'w') as z:
        add(z, f'{folder}/README.txt', readme)
        add(z, f'{folder}/{prog}', exe, exe=True)
        if mac:
            add(z, f'{folder}/Chord Diagram Studio.app/Contents/Info.plist', PLIST.format(v=VERSION))
            add(z, f'{folder}/Chord Diagram Studio.app/Contents/MacOS/chordstudio', exe, exe=True)
        for f in EXAMPLES:
            add(z, f'{folder}/{f}', open(os.path.join(ROOT, 'example', f), 'rb').read())
        for f in SOURCE:
            add(z, f'{folder}/source/{f}', open(os.path.join(ROOT, f), 'rb').read(), exe=f.endswith('.sh'))
    print('wrote', path)
