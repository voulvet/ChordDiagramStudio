CHORD DIAGRAM STUDIO  (Mac, Apple chip)
=======================================

FIRST TIME ONLY
  macOS blocks apps downloaded from outside the App Store. After unzipping,
  open Terminal and run (adjust the path if you moved the folder):

      xattr -cr ~/Downloads/ChordDiagramStudio

  Then double-click "Chord Diagram Studio.app".
  (Optional: drag the app into your Applications folder.)

USING IT
  The app opens a page in your web browser. Everything runs on your own
  computer; nothing is uploaded to the internet.

  1 Data        Choose the Excel file. Sheet 1 = Spearman r matrix,
                sheet 2 = p-value matrix. Row 1 and column A hold the
                CATEGORY name of each variable. Set the p threshold (0.05).
  2 Combine     "Combine categories": every category containing a keyword
                (e.g. "day") becomes one category with the name you give.
                "Keep apart": categories containing the keyword (e.g. "day")
                are not compared with each other, but each is still compared
                with all other categories.
  3 Categories  Untick a category to exclude it. Arrows change the order
                around the ring. Each category keeps its colour.
  4 Diagrams    "One diagram", or "Series": one diagram per ticked category
                (e.g. one per day), each showing only that category's
                correlations. Title and file-name prefix are optional.
  5 Strength    Cut-offs for very strong / strong / moderate (|r|, positive
                or negative). Default 0.80 / 0.60 / 0.40; below = weak.

  Press Generate. For every diagram you see the figure plus counts:
  total, positive, negative, and per strength category; per category pair
  and per category tables are under "Counts per ...".
  Download PNG (300 dpi), SVG (vector) and an Excel file with all counts,
  or "Download all (zip)".

  Press "Quit" in the page when you are done.

ALWAYS APPLIED
  - only correlations with p <= threshold
  - no correlations within the same category
  - one thin chord per correlation, soft blue (-1) / white (0) / soft red (+1)
  - equal-size pastel ring segments, smooth ribbons that do not weave

COMMAND LINE (optional)
  The file "chordstudio" is the same program for Terminal use:
      ./chordstudio                      opens the interface
      ./chordstudio -apart day data.xlsx
      ./chordstudio -merge "day=Pain scoring" data.xlsx
      ./chordstudio -series "Day 1 ...,Day 2 ..." -apart day data.xlsx
      ./chordstudio -exclude "Biochemistry" -alpha 0.01 data.xlsx

  The "source" folder contains the Go source code.

**AI DISCLOSURE** 
The source code of Chord Diagram Studio was generated with Claude (Anthropic), an AI assistant, based on the author's specifications. 
The author defined the requirements and design, and tested and validated the program's output against independent calculations. 
The program draws figures directly from the input data; it does not use AI to create or modify them.
