#!/usr/bin/env python3
"""Render a BBCA analysis JSON snapshot without recalculating its financial values."""

import argparse
import json
import os
import sys
import tempfile
from pathlib import Path
from urllib.parse import urlsplit
from xml.sax.saxutils import escape, quoteattr

try:
    import reportlab
    from reportlab.lib import colors
    from reportlab.lib.enums import TA_RIGHT
    from reportlab.lib.pagesizes import A4
    from reportlab.lib.styles import ParagraphStyle
    from reportlab.pdfbase import pdfmetrics
    from reportlab.pdfbase.ttfonts import TTFont
    from reportlab.platypus import (
        KeepTogether, LongTable, Paragraph, SimpleDocTemplate, TableStyle,
    )
except ModuleNotFoundError as exc:
    if exc.name != "reportlab":
        raise
    sys.exit("BBCA PDF requires ReportLab. See data/README.md for installation.")

NAVY = colors.HexColor("#123047")
TEAL = colors.HexColor("#006F67")
MUTED = colors.HexColor("#465767")
LINE = colors.HexColor("#CFDBE3")
PALE = colors.HexColor("#EDF5F6")
WHITE = colors.white
WIDTH = A4[0] - 88
PUNCTUATION = str.maketrans({"–": "-", "—": "-", "−": "-", "‑": "-", " ": " "})
FONT = "Report Sans"
BOLD = "Report Sans Bold"
FONT_DIRECTORY = Path(reportlab.__file__).parent / "fonts"
pdfmetrics.registerFont(TTFont(FONT, str(FONT_DIRECTORY / "Vera.ttf")))
pdfmetrics.registerFont(TTFont(BOLD, str(FONT_DIRECTORY / "VeraBd.ttf")))
pdfmetrics.registerFontFamily(FONT, normal=FONT, bold=BOLD)

STYLES = {
    "title": ParagraphStyle("title", fontName=BOLD, fontSize=31, leading=37, spaceAfter=9, textColor=NAVY),
    "deck": ParagraphStyle("deck", fontSize=13, leading=19, textColor=MUTED, spaceAfter=16),
    "section": ParagraphStyle("section", fontName=BOLD, fontSize=22, leading=26,
                              spaceBefore=11, spaceAfter=9, textColor=NAVY, keepWithNext=True),
    "heading": ParagraphStyle("heading", fontName=BOLD, fontSize=12, leading=15,
                              spaceBefore=9, spaceAfter=3, textColor=NAVY, keepWithNext=True),
    "year": ParagraphStyle("year", fontName=BOLD, fontSize=16, leading=21,
                           spaceBefore=13, spaceAfter=6, textColor=NAVY, keepWithNext=True),
    "eyebrow": ParagraphStyle("eyebrow", fontName=BOLD, fontSize=9, leading=12,
                              spaceAfter=5, textColor=TEAL, keepWithNext=True),
    "body": ParagraphStyle("body", fontSize=10, leading=14, spaceAfter=7, textColor=NAVY),
    "note": ParagraphStyle("note", fontSize=8.3, leading=11.3, spaceAfter=5, textColor=MUTED),
    "sources": ParagraphStyle("sources", fontSize=8, leading=11, spaceAfter=7, textColor=MUTED),
    "cell": ParagraphStyle("cell", fontSize=8.1, leading=10.6, textColor=NAVY),
    "number": ParagraphStyle("number", fontSize=8.1, leading=10.6, textColor=NAVY, alignment=TA_RIGHT),
    "header": ParagraphStyle("header", fontName=BOLD, fontSize=8, leading=10, textColor=WHITE),
}
for style in STYLES.values():
    if style.fontName == "Helvetica":
        style.fontName = FONT

def plain(value):
    return escape(str(value).translate(PUNCTUATION))


def link(reference):
    label, url = reference["text"], reference["url"]
    if url.startswith("#") or urlsplit(url).scheme == "https":
        return f'<link href={quoteattr(url)} color="#006F67">{plain(label)}</link>'
    if url == "bbca-fy2025-evidence.md":
        return "bbca-fy2025-evidence.pdf"
    return plain(label)


def cell_content(cell):
    label = f'<b>{plain(cell["label"])}.</b> ' if cell.get("label") else ""
    return label + " ".join([plain(cell.get("text", "")), *(link(ref) for ref in cell.get("links", []))]).strip()


class PDFDocument(SimpleDocTemplate):
    def afterFlowable(self, flowable):
        anchor = getattr(flowable, "anchor", "")
        if anchor:
            self.canv.bookmarkHorizontalAbsolute(anchor, self.frame._y + flowable.height)
            if not anchor.startswith("calc-"):
                self.canv.addOutlineEntry(flowable.getPlainText(), anchor, level=0)


class AnalysisPDF:
    def __init__(self, report, view):
        self.view = view
        section_key, self.title = {
            "brief": ("brief_sections", "BBCA fundamental analysis"),
            "evidence": ("evidence_sections", "BBCA evidence companion"),
            "yearly": ("yearly_sections", "BBCA Year by Year"),
        }[view]
        self.sections = report[section_key]
        self.story = []

    def paragraph(self, text, style="body"):
        self.story.append(Paragraph(text, STYLES[style]))

    def table(self, table):
        headers = table["headers"]
        if not table.get("rows"):
            return
        if table.get("title"):
            self.paragraph(plain(table["title"]), "heading")
        widths = {
            "Catalog rules": [.28, .20, .25, .27],
            "Evidence and gaps": [.19, .16, .50, .15],
            "Source candidates": [.29, .16, .55],
            "Reviewed value or reference": [.40, .60],
            "Locator": [.47, .53],
            "Value and unit": [.30, .12, .16, .15, .27],
            "Note": [.14, .20, .66],
            "Gap": [.21, .15, .26, .38],
            "Profit YoY": [.12, .17, .18, .17, .19, .17],
            "FY2025": [.36, .22, .15, .135, .135],
        }
        fractions = next((v for k, v in widths.items() if k in headers), [1 / len(headers)] * len(headers))
        cells = [[Paragraph(plain(header), STYLES["header"]) for header in headers]]
        for row in table["rows"]:
            cells.append([Paragraph(cell_content(cell), STYLES["cell"]) for cell in row])
        grid = LongTable(cells, colWidths=[WIDTH * fraction for fraction in fractions],
                         repeatRows=1, hAlign="LEFT", spaceAfter=10)
        grid.setStyle(TableStyle([
            ("BACKGROUND", (0, 0), (-1, 0), NAVY),
            ("ROWBACKGROUNDS", (0, 1), (-1, -1), [WHITE, PALE]),
            ("LINEBELOW", (0, 0), (-1, 0), 0.7, TEAL),
            ("LINEBELOW", (0, -1), (-1, -1), 0.5, LINE),
            ("VALIGN", (0, 0), (-1, -1), "TOP"),
            ("TOPPADDING", (0, 0), (-1, -1), 4),
            ("BOTTOMPADDING", (0, 0), (-1, -1), 4),
        ]))
        self.story.append(grid)

    def render(self, destination):
        edition = "1998-2026" if self.view == "yearly" else "FY2025"
        self.paragraph(plain(edition), "eyebrow")
        self.paragraph(plain(self.title), "title")
        for section in self.sections:
            anchor = section.get("id", "")
            year = anchor.startswith("year-")
            start = len(self.story)
            if section.get("title"):
                heading_style = "year" if year else "heading" if anchor.startswith("calc-") else "section"
                self.paragraph(plain(section["title"]), heading_style)
                self.story[-1].anchor = anchor
            for cell in section.get("paragraphs", []):
                style = "sources" if year and not cell.get("text") else "note" if anchor.startswith("calc-") else "body"
                self.paragraph(cell_content(cell), style)
            for table in section.get("tables", []):
                self.table(table)
            if year:
                self.story[start:] = [KeepTogether(self.story[start:])]

        doc = PDFDocument(str(destination), pagesize=A4, leftMargin=44, rightMargin=44,
                          topMargin=49, bottomMargin=47, title=self.title + " - " + edition,
                          author="Alcazar")

        def page(canvas, _doc):
            canvas.setStrokeColor(LINE)
            canvas.line(44, A4[1] - 35, A4[0] - 44, A4[1] - 35)
            canvas.line(44, 35, A4[0] - 44, 35)
            canvas.setFont(FONT, 8)
            canvas.setFillColor(MUTED)
            canvas.drawString(44, 23, f"BBCA  /  {edition}  /  {self.view.upper()}")
            canvas.drawRightString(A4[0] - 44, 23, str(canvas.getPageNumber()))

        doc.build(self.story, onFirstPage=page, onLaterPages=page)
        if self.view == "brief" and doc.page > 6:
            raise ValueError(f"BBCA brief exceeds its six-page budget ({doc.page} pages)")


def main():
    cli = argparse.ArgumentParser(description=__doc__)
    cli.add_argument("report", type=Path, help="BBCA analysis JSON")
    cli.add_argument("output", type=Path, help="destination PDF")
    cli.add_argument("--view", choices=("brief", "evidence", "yearly"), default="brief")
    args = cli.parse_args()
    with args.report.open(encoding="utf-8") as stream:
        report = json.load(stream)
    pdf = AnalysisPDF(report, args.view)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".bbca-pdf-", dir=args.output.parent) as directory:
        draft = Path(directory) / "report.pdf"
        pdf.render(draft)
        os.replace(draft, args.output)
    print(args.output)


if __name__ == "__main__":
    main()
