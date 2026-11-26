#!/usr/bin/env python3
"""Render a BBCA analysis JSON snapshot without recalculating its financial values."""

import argparse
import json
import os
import sys
import tempfile
from io import BytesIO
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
        LongTable, PageBreak, Paragraph, SimpleDocTemplate, Spacer, Table, TableStyle,
    )
except ModuleNotFoundError as exc:
    if exc.name != "reportlab":
        raise
    sys.exit("BBCA PDF requires ReportLab. See data/README.md for installation.")

NAVY = colors.HexColor("#123047")
TEAL = colors.HexColor("#006F67")
MUTED = colors.HexColor("#465767")
LINE = colors.HexColor("#CFDBE3")
WHITE = colors.white
WIDTH = A4[0] - 88
YEARLY_WIDTH = WIDTH - 12
PUNCTUATION = str.maketrans({"–": "-", "—": "-", "−": "-", "‑": "-", " ": " "})
FONT = "Report Sans"
BOLD = "Report Sans Bold"
FONT_DIRECTORY = Path(reportlab.__file__).parent / "fonts"
pdfmetrics.registerFont(TTFont(FONT, str(FONT_DIRECTORY / "Vera.ttf")))
pdfmetrics.registerFont(TTFont(BOLD, str(FONT_DIRECTORY / "VeraBd.ttf")))
pdfmetrics.registerFontFamily(FONT, normal=FONT, bold=BOLD)

STYLES = {
    "cover_title": ParagraphStyle("cover_title", fontName=BOLD, fontSize=40, leading=45, spaceAfter=14, textColor=NAVY),
    "deck": ParagraphStyle("deck", fontSize=13, leading=19, textColor=MUTED, spaceAfter=16),
    "heading": ParagraphStyle("heading", fontName=BOLD, fontSize=12, leading=15,
                              spaceBefore=9, spaceAfter=3, textColor=NAVY, keepWithNext=True),
    "year_number": ParagraphStyle("year_number", fontName=BOLD, fontSize=25, leading=29,
                                   textColor=NAVY, keepWithNext=True),
    "year_headline": ParagraphStyle("year_headline", fontName=BOLD, fontSize=13, leading=17,
                                     textColor=NAVY, keepWithNext=True),
    "year_glossary": ParagraphStyle("year_glossary", fontName=BOLD, fontSize=25, leading=30,
                                     spaceAfter=3, textColor=NAVY, keepWithNext=True),
    "eyebrow": ParagraphStyle("eyebrow", fontName=BOLD, fontSize=9, leading=12,
                              spaceAfter=5, textColor=TEAL, keepWithNext=True),
    "sources": ParagraphStyle("sources", fontSize=8, leading=11, spaceAfter=7, textColor=MUTED),
    "year_header": ParagraphStyle("year_header", fontName=BOLD, fontSize=7.8, leading=10, textColor=MUTED),
    "year_cell": ParagraphStyle("year_cell", fontSize=8.15, leading=10.5, textColor=NAVY),
    "year_number_cell": ParagraphStyle("year_number_cell", fontSize=8.15, leading=10.5,
                                        textColor=NAVY, alignment=TA_RIGHT),
    "year_group": ParagraphStyle("year_group", fontName=BOLD, fontSize=8.3, leading=10.8,
                                  textColor=TEAL),
    "year_bullet": ParagraphStyle("year_bullet", fontSize=9.2, leading=12.1,
                                   leftIndent=10, firstLineIndent=0, spaceAfter=4, textColor=MUTED),
}
STYLES["year_numeric_header"] = ParagraphStyle("year_numeric_header", parent=STYLES["year_header"], alignment=TA_RIGHT)
STYLES["year_current_header"] = ParagraphStyle("year_current_header", parent=STYLES["year_numeric_header"], textColor=TEAL)
STYLES["year_current"] = ParagraphStyle("year_current", parent=STYLES["year_number_cell"], fontName=BOLD)
for style in STYLES.values():
    if style.fontName == "Helvetica":
        style.fontName = FONT

def plain(value):
    return escape(str(value).translate(PUNCTUATION))


def link(reference):
    label, url = reference["text"], reference["url"]
    if url.startswith("#") or urlsplit(url).scheme == "https":
        return f'<link href={quoteattr(url)} color="#006F67">{plain(label)}</link>'
    return plain(label)


def cell_content(cell, separator=" "):
    label = f'<b>{plain(cell["label"])}.</b> ' if cell.get("label") else ""
    references = separator.join(link(ref) for ref in cell.get("links", []))
    return label + " ".join(filter(None, (plain(cell.get("text", "")), references)))


class PDFDocument(SimpleDocTemplate):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.page_starts = {}

    def afterFlowable(self, flowable):
        anchor = getattr(flowable, "anchor", "")
        if anchor:
            self.page_starts[anchor] = self.page
            height = getattr(flowable, "height", getattr(flowable, "_height", 0))
            self.canv.bookmarkHorizontalAbsolute(anchor, self.frame._y + height)
            if not anchor.startswith("calc-"):
                title = flowable.getPlainText() if hasattr(flowable, "getPlainText") else getattr(flowable, "anchor_title", anchor)
                self.canv.addOutlineEntry(title, anchor, level=0)


def legacy_yearly_sections(sections):
    """Adapt archived yearly snapshots, preserving their recorded section order."""
    adapted = []
    for section in sections:
        anchor = section.get("id", "")
        period = ""
        if anchor.startswith("year-"):
            period = section.get("title", "").partition(" — ")[0]
        kind = {
            "yearly-cover": "cover", "year-2026": "interim", "yearly-glossary": "glossary",
        }.get(anchor, "annual" if period else "analysis")
        tables = []
        for table in section.get("tables", []):
            headers = table["headers"]
            if headers[0] == "Measure" and len(headers) == 4:
                numeric = [1, 2] if headers[-1] == "Scope" else [1, 2, 3]
                table = dict(table, numeric_columns=numeric, current_column=2)
            tables.append(table)
        adapted.append(dict(section, kind=kind, period_label=period, tables=tables))

    contents_targets = {section.get("title"): section["id"] for section in adapted if section.get("id")}
    for label, kind in (("Interim update", "interim"), ("Annual comparison", "annual")):
        target = next((section["id"] for section in adapted if section["kind"] == kind), None)
        if target:
            contents_targets[label] = target
    for section in adapted:
        if section["kind"] != "cover":
            continue
        for index, table in enumerate(section["tables"]):
            if table["headers"] != ["Section", "Coverage", "Page"]:
                continue
            rows = []
            for label, coverage, _ in table["rows"]:
                title = label["text"]
                if title in contents_targets:
                    label = {"links": [{"text": title, "url": "#" + contents_targets[title]}]}
                rows.append([label, coverage])
            section["tables"][index] = dict(table, headers=["Section", "Coverage"], rows=rows)
    return adapted


def fill_archived_layout(sections):
    """Resolve metadata absent from snapshots written before Go prepared the layout."""
    for section in sections:
        if "layout" in section:
            continue
        section["layout"] = {
            "takeaways": section["kind"] in ("annual", "interim", "analysis"),
            "source_rows": [i for i, cell in enumerate(section.get("paragraphs", [])) if cell.get("label") == "Sources"],
        }
        for table in section.get("tables", []):
            headers = table["headers"]
            roles = ["numeric" if i in table.get("numeric_columns", []) else "" for i in range(len(headers))]
            current = table.get("current_column", 0)
            if 0 < current < len(roles):
                roles[current] = "current"
            table["layout"] = {
                "contents": section["kind"] == "cover" and headers == ["Section", "Coverage"],
                "column_roles": roles,
                "group_rows": [
                    i for i, row in enumerate(table["rows"] or [])
                    if len(row) > 1 and row[0].get("text") and all(
                        not cell.get("text") and not cell.get("links") for cell in row[1:]
                    )
                ],
            }


def yearly_paragraphs(section):
    body, sources = [], []
    source_rows = set(section["layout"].get("source_rows", []))
    for index, cell in enumerate(section.get("paragraphs", [])):
        (sources if index in source_rows else body).append(cell)
    return body, sources


class AnalysisPDF:
    def __init__(self, report):
        self.sections = report["yearly_sections"]
        template = report.get("yearly_template")
        if template is None:
            template = {"id": "standard-1", "title": "BBCA Year by Year",
                        "footer": "BBCA fundamentals · year-by-year comparison"}
            self.sections = legacy_yearly_sections(self.sections)
        if template["id"] != "standard-1":
            raise ValueError(f"unsupported yearly template {template['id']!r}")
        fill_archived_layout(self.sections)
        self.title = template["title"]
        self.footer = template["footer"]
        self.story = []

    def paragraph(self, text, style):
        self.story.append(Paragraph(text, STYLES[style]))

    def yearly_table(self, table, contents=False):
        if not table.get("rows"):
            return
        headers = table["headers"]
        layout = table["layout"]
        roles = layout["column_roles"]
        widths = {2: [.25, .75], 3: [.49, .15, .36], 4: [.49, .185, .185, .14]}
        fractions = [.49, .36, .15] if contents else widths.get(len(headers), [1 / len(headers)] * len(headers))
        header_style = {"": "year_header", "numeric": "year_numeric_header", "current": "year_current_header"}
        value_style = {"": "year_cell", "numeric": "year_number_cell", "current": "year_current"}

        caption = table.get("title") if not contents else None
        cells = []
        if caption:
            cells.append([Paragraph(plain(caption), STYLES["heading"]), *([""] * (len(headers) - 1))])
        header_row = len(cells)
        cells.append([Paragraph(plain(header), STYLES[header_style[role]]) for header, role in zip(headers, roles)])
        group_rows = set(layout.get("group_rows", []))
        for row_index, row in enumerate(table["rows"]):
            if row_index in group_rows:
                cells.append([Paragraph(plain(row[0]["text"]), STYLES["year_group"]), *([""] * (len(headers) - 1))])
                continue
            cells.append([
                Paragraph(plain(cell.get("text", "")) if role else cell_content(cell), STYLES[value_style[role]])
                for cell, role in zip(row, roles)
            ])
        grid = LongTable(cells, colWidths=[YEARLY_WIDTH * fraction for fraction in fractions],
                         repeatRows=(header_row,), hAlign="LEFT", spaceAfter=7)
        style = [
            ("BACKGROUND", (0, 0), (-1, 0), WHITE),
            ("LINEBELOW", (0, header_row), (-1, header_row), 0.7, LINE),
            ("LINEBELOW", (0, -1), (-1, -1), 0.5, LINE),
            ("VALIGN", (0, 0), (-1, -1), "TOP"),
            ("TOPPADDING", (0, 0), (-1, -1), 4),
            ("BOTTOMPADDING", (0, 0), (-1, -1), 4),
            ("LINEBELOW", (0, header_row + 1), (-1, -1), 0.25, LINE),
        ]
        if caption:
            style.extend([
                ("SPAN", (0, 0), (-1, 0)),
                ("LEFTPADDING", (0, 0), (0, 0), 0),
                ("TOPPADDING", (0, 0), (0, 0), 5),
            ])
        if contents:
            style.extend([
                ("TOPPADDING", (0, 1), (-1, -1), 10),
                ("BOTTOMPADDING", (0, 1), (-1, -1), 10),
                ("LINEBELOW", (0, 1), (-1, -1), 0.45, LINE),
            ])
        for index in sorted(group_rows):
            row_index = header_row + 1 + index
            style.extend([
                ("SPAN", (0, row_index), (-1, row_index)),
                ("LINEBELOW", (0, row_index), (-1, row_index), 0.4, LINE),
                ("TOPPADDING", (0, row_index), (-1, row_index), 7),
                ("BOTTOMPADDING", (0, row_index), (-1, row_index), 2),
            ])
        grid.setStyle(TableStyle(style))
        self.story.append(grid)

    def yearly_cover(self, section, page_starts):
        self.story.append(Spacer(1, 72))
        self.paragraph(plain(self.title), "cover_title")
        self.story[-1].anchor = section["id"]
        body, sources = yearly_paragraphs(section)
        for cell in body:
            self.paragraph(cell_content(cell), "deck")
        for table in section.get("tables", []):
            layout = table["layout"]
            if not layout.get("contents"):
                self.yearly_table(table)
                continue
            self.paragraph(plain(table["title"]), "eyebrow")
            rows = []
            for row in table["rows"]:
                refs = row[0].get("links", [])
                page = page_starts.get(refs[0]["url"][1:], 0) if refs else None
                rows.append([*row, {"text": f"{page:02d}" if page is not None else ""}])
            self.yearly_table(dict(table, headers=[*table["headers"], "Page"], rows=rows,
                                   layout=dict(layout, column_roles=[*layout["column_roles"], "numeric"])), contents=True)
        for cell in sources:
            self.paragraph(cell_content(cell, " · "), "sources")

    def period_heading(self, title, period):
        headline = title.removeprefix(period + " — ")
        label_width = pdfmetrics.stringWidth(period, BOLD, STYLES["year_number"].fontSize)
        width = min(YEARLY_WIDTH * .43, max(92, label_width + 14))
        heading = Table([
            [Paragraph(plain(period), STYLES["year_number"]), Paragraph(plain(headline), STYLES["year_headline"])],
        ], colWidths=[width, YEARLY_WIDTH - width], hAlign="LEFT")
        heading.setStyle(TableStyle([
            ("VALIGN", (0, 0), (-1, -1), "MIDDLE"),
            ("LEFTPADDING", (0, 0), (-1, -1), 0),
            ("RIGHTPADDING", (0, 0), (-1, -1), 8),
            ("TOPPADDING", (0, 0), (-1, -1), 0),
            ("BOTTOMPADDING", (0, 0), (-1, -1), 5),
        ]))
        heading.anchor_title = title
        heading.keepWithNext = True
        return heading

    def yearly_section(self, section, page_starts):
        kind = section["kind"]
        if kind == "cover":
            self.yearly_cover(section, page_starts)
            return

        title = section["title"]
        period = section.get("period_label")
        heading = self.period_heading(title, period) if period else Paragraph(plain(title), STYLES["year_glossary"])
        heading.anchor = section["id"]
        self.story.append(heading)
        takeaways = section["layout"].get("takeaways", False)
        style = "year_bullet" if takeaways else "deck"
        body, sources = yearly_paragraphs(section)
        for cell in body:
            self.story.append(Paragraph(cell_content(cell), STYLES[style], bulletText="•" if takeaways else None))
        if takeaways and section.get("tables"):
            self.story.append(Spacer(1, 8))
        for table in section.get("tables", []):
            self.yearly_table(table)
        for cell in sources:
            self.paragraph(cell_content(cell, " · "), "sources")

    def build_yearly(self, destination, page_starts, page_count=None):
        self.story = []
        for index, section in enumerate(self.sections):
            if index:
                self.story.append(PageBreak())
            self.yearly_section(section, page_starts)

        doc = PDFDocument(destination, pagesize=A4, leftMargin=44, rightMargin=44,
                          topMargin=49, bottomMargin=47, title=self.title, author="Alcazar")

        def year_page(canvas, _doc):
            canvas.setStrokeColor(TEAL)
            canvas.setLineWidth(1.35)
            canvas.line(44, A4[1] - 35, A4[0] - 44, A4[1] - 35)
            canvas.setStrokeColor(LINE)
            canvas.setLineWidth(0.6)
            canvas.line(44, 35, A4[0] - 44, 35)
            canvas.setFont(FONT, 8)
            canvas.setFillColor(MUTED)
            canvas.drawString(44, 23, self.footer)
            if page_count is not None:
                canvas.drawRightString(A4[0] - 44, 23, f"{canvas.getPageNumber():02d} / {page_count:02d}")

        doc.build(self.story, onFirstPage=year_page, onLaterPages=year_page)
        return doc.page_starts, doc.page

    def render(self, destination):
        with BytesIO() as draft:
            page_starts, page_count = self.build_yearly(draft, {})
        if self.build_yearly(str(destination), page_starts, page_count) != (page_starts, page_count):
            raise ValueError("yearly contents changed document pagination")


def main():
    cli = argparse.ArgumentParser(description=__doc__)
    cli.add_argument("report", type=Path, help="BBCA analysis JSON")
    cli.add_argument("output", type=Path, help="destination PDF")
    cli.add_argument("--view", choices=("yearly",), default="yearly")
    args = cli.parse_args()
    with args.report.open(encoding="utf-8") as stream:
        report = json.load(stream)
    pdf = AnalysisPDF(report)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".bbca-pdf-", dir=args.output.parent) as directory:
        draft = Path(directory) / "report.pdf"
        pdf.render(draft)
        os.replace(draft, args.output)
    print(args.output)


if __name__ == "__main__":
    main()
