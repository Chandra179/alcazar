#!/usr/bin/env python3
"""Render a BBCA analysis JSON snapshot without recalculating its financial values."""

import argparse
import json
import os
import sys
import tempfile
from decimal import Decimal
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
        KeepTogether, LongTable, PageBreak, Paragraph, SimpleDocTemplate, Spacer, TableStyle,
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
    "title": ParagraphStyle("title", fontName=BOLD, fontSize=39, leading=46, textColor=NAVY),
    "deck": ParagraphStyle("deck", fontSize=13, leading=19, textColor=MUTED, spaceAfter=16),
    "section": ParagraphStyle("section", fontName=BOLD, fontSize=22, leading=26,
                              spaceBefore=11, spaceAfter=9, textColor=NAVY, keepWithNext=True),
    "heading": ParagraphStyle("heading", fontName=BOLD, fontSize=12, leading=15,
                              spaceBefore=9, spaceAfter=3, textColor=NAVY, keepWithNext=True),
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

# Formulas outside the catalog. The calculation and its actual inputs remain in Go.
METHODS = {
    "bank-net-income-to-operating-income": "Net income / (net interest and sharia income + other operating income) × 100",
    "pretax-bridge-residual": "Net interest and sharia income + other operating income + net insurance income - impairment losses + signed operating expenses - pretax income",
    "cash-reconciliation-residual": "Opening cash + sum of operating, investing and financing flows + FX effect - closing cash",
    "nominal-shareholder-return": "((Ending close + cash dividends paid) / opening close - 1) × 100",
    "fiscal-dividend-payout": "Fiscal dividend per share / basic EPS × 100",
    "dividend-value": "Present value of five annual dividends plus terminal value D5 × (1 + terminal growth) / (cost of equity - terminal growth), discounted to today",
    "sensitivity": "Dividend discount model with base-case initial growth and the displayed terminal growth and cost of equity",
    "margin-of-safety": "(1 - year-end close / conservative estimated value) × 100",
}


def plain(value):
    return escape(str(value).translate(PUNCTUATION))


def link(label, url):
    if not url or urlsplit(url).scheme != "https":
        return plain(label)
    return f'<link href={quoteattr(url)} color="#006F67">{plain(label)}</link>'


def number(value, digits=None):
    if value is None:
        return "-"
    if digits is not None:
        return f"{float(value):,.{digits}f}"
    decimal = Decimal(str(value))
    if decimal == decimal.to_integral_value():
        return f"{int(decimal):,}"
    return f"{decimal:,.6f}".rstrip("0").rstrip(".")


def words(key):
    return str(key).replace("_", " ").replace("-", " ")


def metric_text(metric):
    if "value" not in metric:
        return metric.get("reason") or words(metric.get("status", "unavailable"))
    value = number(metric["value"], 2)
    unit = metric.get("unit", "")
    if unit in {"percent", "%"}:
        return value + "%"
    if unit in {"ratio", "×"}:
        return value + "×"
    if unit == "currency per share":
        unit = "IDR/share"
    return f"{value} {unit}".strip()


class PDFDocument(SimpleDocTemplate):
    def afterFlowable(self, flowable):
        anchor = getattr(flowable, "outline", "")
        if anchor:
            self.canv.bookmarkPage(anchor)
            self.canv.addOutlineEntry(flowable.getPlainText(), anchor, level=0)


class AnalysisPDF:
    def __init__(self, report):
        self.report = report
        self.sources = report["sources"]
        self.citations = {key: i for i, key in enumerate(sorted(self.sources), 1)}
        self.rules = {item["rule"]["id"]: item["rule"] for item in report["rules"]}
        self.facts = {
            fact["id"]: fact
            for row in report["history"] + report["interim"]
            for fact in row["facts"].values()
        }
        self.story = []

    def paragraph(self, text, style="body", markup=False):
        self.story.append(Paragraph(text if markup else plain(text), STYLES[style]))

    def section(self, title, anchor, new_page=False):
        if new_page:
            self.story.append(PageBreak())
        self.paragraph(title, "section")
        self.story[-1].outline = anchor

    def table(self, headers, rows, widths, numeric=()):
        cells = [[Paragraph(plain(header), STYLES["header"]) for header in headers]]
        for row in rows:
            cells.append([
                Paragraph(text, STYLES["number" if i in numeric else "cell"])
                for i, text in enumerate(row)
            ])
        table = LongTable(cells, colWidths=[WIDTH * fraction for fraction in widths],
                          repeatRows=1, hAlign="LEFT", spaceAfter=12)
        table.setStyle(TableStyle([
            ("BACKGROUND", (0, 0), (-1, 0), NAVY),
            ("ROWBACKGROUNDS", (0, 1), (-1, -1), [WHITE, PALE]),
            ("LINEBELOW", (0, 0), (-1, 0), 0.7, TEAL),
            ("LINEBELOW", (0, -1), (-1, -1), 0.5, LINE),
            ("VALIGN", (0, 0), (-1, -1), "TOP"),
            ("TOPPADDING", (0, 0), (-1, -1), 4),
            ("BOTTOMPADDING", (0, 0), (-1, -1), 4),
        ]))
        self.story.append(table)

    def cited(self, key, short=False):
        source = self.sources[key]
        ref = f"[{self.citations[key]}]"
        if not short:
            ref += f' {source["title"]}, {source["locator"]}'
        return link(ref, source["url"])

    def assessment(self, record):
        self.paragraph(record["finding"])
        if record.get("gap"):
            self.paragraph(record["gap"], "note")
        if record.get("sources"):
            self.paragraph(" &nbsp; ".join(self.cited(key) for key in record["sources"]), "sources", True)

    def fact_link(self, fact, label=None):
        if not fact:
            return "-"
        page = fact.get("page_number")
        locator = fact.get("source_locator", "")
        url = fact.get("source_url", "")
        if page and url:
            url = url.split("#", 1)[0] + f"#page={page}"
        if label is None:
            name = Path(fact.get("document_path", "")).stem.replace("_", " ")
            label = ", ".join(str(v) for v in (name, f"PDF p. {page}" if page else "", locator) if v)
        return link(label, url)

    def fact_money(self, fact):
        if not fact:
            return "-"
        scales = {"units": 1, "idr": 1, "thousands": 1e3, "millions": 1e6, "billions": 1e9}
        if fact["unit"] not in scales:
            return "-"
        value = float(fact["normalized_value"]) * scales[fact["unit"]] / 1e12
        return self.fact_link(fact, number(value, 2))

    def cover(self):
        self.story.append(Spacer(1, 44))
        self.paragraph("PT BANK CENTRAL ASIA TBK   /   IDX: BBCA", "eyebrow")
        self.paragraph("Fundamental analysis", "title")
        self.paragraph("FY2025 audited year, 1998-2025 history and the 2026 interim update", "deck")
        self.story.append(Spacer(1, 16))
        results = self.report["results"]
        ids = ["return-on-assets", "return-on-equity", "price-earnings-ratio", "price-to-book-ratio", "dividend-yield"]
        self.table(["FY2025 measure", "Result"], [[plain(words(id_).capitalize()), plain(metric_text(results.get(id_, {})))] for id_ in ids],
                   [.63, .37], (1,))
        self.paragraph(f'{len(self.rules)} catalog rules. Financial statements, historical valuation, rule assessments and a calculation appendix.')
        self.paragraph("Catalog SHA-256: " + self.report["catalog_hash"], "note")
        if self.report["reviewed_catalog_hash"] != self.report["catalog_hash"]:
            self.paragraph("Reviewed catalog SHA-256: " + self.report["reviewed_catalog_hash"], "note")
            self.paragraph("Catalog changed since the FY2025 evidence was reviewed. Check revised and unreviewed entries.", "note")
        self.story.append(Spacer(1, 26))
        self.paragraph("CONTENTS", "eyebrow")
        for anchor, title in (("analysis", "FY2025 analysis"), ("history", "Financial history and interim update"),
                              ("banking", "Banking disclosures and peers"), ("rules", "Rule-by-rule assessment"),
                              ("calculations", "Calculation details"), ("sources", "Source register")):
            self.paragraph(f'<link href="#{anchor}" color="#006F67">{plain(title)}</link>', markup=True)

    def main_report(self):
        self.section("FY2025 analysis", "analysis", new_page=True)
        for period in self.report["sections"]:
            self.paragraph(period["title"], "heading")
            self.assessment(period)
        self.section("Statement and valuation results", "results")
        results = self.report["results"]
        ids = [key for key in results if not key.startswith("sensitivity-")]
        self.table(["Measure", "Result"], [
            [plain(words(key).capitalize()), plain(metric_text(results[key]))] for key in ids
        ], [.62, .38], (1,))
        self.paragraph("Dividend value sensitivity (IDR per share)", "heading")
        self.table(["Cost of equity", "Terminal growth 2%", "3%", "4%"], [
            [plain(f"{rate}%")] + [
                plain(number(results.get(f"sensitivity-{rate}-{growth}", {}).get("value"), 2))
                for growth in (2, 3, 4)
            ]
            for rate in (10, 11, 12)
        ], [.28, .24, .24, .24], (1, 2, 3))

    def history(self):
        self.section("Annual financial history", "history")
        self.paragraph("Amounts in IDR trillion. Basic EPS is shown in IDR on the October 2021 share basis. "
                       "Historical highlights are rounded. ROA and ROE use average opening and closing balances. A dash indicates a data gap.", "note")
        eps = {o["period"]: o for o in self.report["observations"]
               if o["key"] == "Basic EPS on 2025 share basis" and o["subject"] == "BBCA"
               and o["basis"] == "consolidated" and o["unit"] == "IDR/share"}
        keys = ("total_assets", "common_shareholders_equity", "shareholders_equity", "net_earnings",
                "net_income_available_to_common_shareholders")
        rows = []
        for row in self.report["history"]:
            year = row["period"]
            values = row["values"]
            eps_evidence = eps.get(year)
            eps_fact = row["facts"].get("reported_earnings_per_share")
            if eps_evidence:
                eps_text = link(number(eps_evidence["value"], 2), self.sources[eps_evidence["source"]]["url"])
            else:
                eps_text = self.fact_link(eps_fact, number(eps_fact["normalized_value"], 2)) if eps_fact else "-"
            rows.append([plain(year)] + [self.fact_money(row["facts"].get(key)) for key in keys] + [
                eps_text, plain(number(values.get("return-on-assets", {}).get("value"), 2)),
                plain(number(values.get("return-on-equity", {}).get("value"), 2)),
            ])
        self.table(["FY", "Assets", "Parent equity", "Group equity", "Group profit", "Parent profit",
                    "Basic EPS", "ROA %", "ROE %"], rows, [.09, .12, .125, .125, .12, .12, .11, .095, .095], range(1, 9))
        self.paragraph("Period and comparability notes", "heading")
        for row in self.report["history"]:
            if row.get("note"):
                self.paragraph(plain(row["period"]) + " &nbsp; " + plain(row["note"]), "note", True)
        self.section("2026 interim snapshot", "interim")
        rows = []
        for row in self.report["interim"]:
            year = row["period"]
            amounts = []
            for key, label in (("total_assets", "Closing assets"), ("shareholders_equity", "Closing equity")):
                fact = row["facts"].get(key)
                if fact:
                    amounts.append(self.fact_money(fact))
                    continue
                observation = next((o for o in self.report["observations"]
                                    if (o["key"], o["period"], o["basis"], o["unit"], o["subject"])
                                    == (label, year, row["basis"], "IDR million", "BBCA")), None)
                amounts.append(link(number(observation["value"] / 1e6, 2), self.sources[observation["source"]]["url"])
                               if observation else "-")
            rows.append([plain(year), plain(row["basis"])] + amounts + [
                self.fact_money(row["facts"].get("net_earnings")), plain(row.get("note", ""))])
        self.table(["Period", "Basis", "Assets (IDR T)", "Equity (IDR T)", "YTD profit (IDR T)", "Coverage"],
                   rows, [.11, .14, .13, .13, .13, .36], (2, 3, 4))

    def banking(self):
        self.section("Banking disclosures and peers", "banking")
        keys = {"Gross loans and financing", "Corporate loans", "Third-party funds", "CASA ratio", "Gross NPL",
                "Loan at risk", "Net interest margin", "Cost to income", "Impairment losses", "Capital adequacy ratio",
                "Liquidity coverage ratio", "Net stable funding ratio", "Loan to deposit ratio"}
        periods = {}
        for o in self.report["observations"]:
            if o["subject"] == "BBCA" and o["period"] in ("2024", "2025") and o["key"] in keys:
                periods.setdefault((o["key"], o["basis"], o["unit"]), {})[o["period"]] = o
        rows = []
        for (key, basis, unit), values in periods.items():
            row = [plain(key), plain(basis), plain(unit)]
            for year in ("2024", "2025"):
                o = values.get(year)
                row.append(link(number(o["value"], 2), self.sources[o["source"]]["url"]) if o else "-")
            rows.append(row)
        self.table(["Measure", "Basis", "Unit", "FY2024", "FY2025"], rows, [.34, .22, .14, .15, .15], (3, 4))
        self.paragraph("FY2025 peer comparison, bank only", "heading")
        rows = []
        for result in self.report["peer_comparisons"]:
            items = {item["subject"]: item for item in result["observations"]}
            row = [plain(result["key"])]
            for stock in ("BBCA", "BBRI", "BMRI"):
                o = items.get(stock)
                row.append(link(number(o["value"], 2) + "%", self.sources[o["source"]]["url"]) if o else "-")
            rows.append(row + [plain(result["status"])])
        self.table(["Measure", "BBCA", "BBRI", "BMRI", "Comparison"], rows, [.31, .13, .13, .13, .30], (1, 2, 3))

    def rule_appendix(self):
        self.section("Rule-by-rule assessment", "rules")
        self.paragraph("Rules appear in catalog order. Formula text is reproduced from the catalog, alongside the applied result or reason the calculation is unavailable.", "note")
        for item in self.report["rules"]:
            start = len(self.story)
            rule = item["rule"]
            self.paragraph(f'{item["number"]}. {rule["id"]}', "heading")
            self.paragraph(f'{rule["type"].upper()}  /  {words(item["status"])}', "eyebrow")
            self.paragraph(rule["description"])
            if rule.get("formula"):
                self.paragraph("Catalog formula: " + rule["formula"], "note")
            if item.get("metric"):
                self.paragraph("Result: " + metric_text(item["metric"]), "note")
            self.assessment(item)
            self.story[start:] = [KeepTogether(self.story[start:])]

    def input_rows(self, inputs, path=""):
        """Yield readable input values and evidence links from the JSON ledger."""
        if isinstance(inputs, list):
            for index, value in enumerate(inputs, 1):
                label = value.get("period") if isinstance(value, dict) else None
                prefix = f"{path} / {label or index}"
                if label:
                    value = {key: val for key, val in value.items() if key != "period"}
                yield from self.input_rows(value, prefix)
        elif not isinstance(inputs, dict):
            formatted = number(inputs) if isinstance(inputs, (float, int)) else str(inputs)
            if isinstance(inputs, (float, int)) and path.rsplit(" / ", 1)[-1] in {"growth", "terminal_growth", "cost_of_equity"}:
                formatted = number(100 * inputs, 2) + "%"
            yield [plain(words(path)), plain(formatted), ""]
        elif "value" in inputs or "normalized_value" in inputs:
            ref = []
            if "source" in inputs:
                ref.append(self.cited(inputs["source"]))
            if "source_url" in inputs:
                ref.append(self.fact_link(inputs))
            for fact in inputs.get("source_facts", []):
                linked = self.facts.get(fact.get("fact_id"))
                if linked:
                    # Some calculated averages reference opening and closing values.
                    amount = number(linked["normalized_value"])
                    detail = f'{fact.get("period", "")}: {amount} {linked["unit"]}'
                    ref.append(plain(detail) + " · " + self.fact_link(fact))
                else:
                    ref.append(self.fact_link(fact))
            value = number(inputs.get("value", inputs.get("normalized_value")))
            unit = " ".join(x for x in (inputs.get("currency", ""), inputs.get("unit", "")) if x)
            basis = inputs.get("basis") or inputs.get("reporting_basis")
            label = words(path)
            if basis:
                label += " (" + basis + ")"
            yield [plain(label), plain(f"{value} {unit}".strip()), "<br/>".join(ref)]
            if inputs.get("inputs"):
                yield from self.input_rows(inputs["inputs"], path)
        else:
            for key, value in inputs.items():
                yield from self.input_rows(value, f"{path} / {key}" if path else key)

    def calculation(self, id_, metric):
        start = len(self.story)
        self.paragraph(f'{words(id_).capitalize()}  ·  {metric.get("period", "")}', "heading")
        self.paragraph(f'{words(metric["status"]).capitalize()}  /  {metric_text(metric)}', "note")
        formula = self.rules.get(metric.get("entity_id"), {}).get("formula")
        if not formula:
            formula = next((value for key, value in METHODS.items() if id_.startswith(key)), None)
        if formula:
            self.paragraph(formula, "note")
        if metric.get("inputs"):
            rows = list(self.input_rows(metric["inputs"]))
            self.table(["Input", "Value", "Source"], rows, [.32, .24, .44])
            if len(rows) <= 7:
                self.story[start:] = [KeepTogether(self.story[start:])]

    def calculations(self):
        self.section("Calculation details", "calculations")
        self.paragraph("FY2025 results, valuation sensitivities and calculations from the other covered periods. "
                       "Inputs retain their source units. Links open the filing page or supplemental disclosure.", "note")
        for id_, metric in self.report["results"].items():
            self.calculation(id_, metric)
        self.section("Other annual and interim calculations", "history-calculations")
        for row in self.report["history"] + self.report["interim"]:
            for id_, metric in row["values"].items():
                headline = self.report["results"].get(id_)
                if "value" in metric and metric != headline:
                    self.calculation(id_, metric)

    def bibliography(self):
        self.section("Source register", "sources")
        self.paragraph("The numbered references below cover supplemental sources and assessment citations. "
                       "The calculation and history tables also link directly to individual filing pages.", "note")
        citations = [self.cited(key) for key in sorted(self.sources)]
        middle = (len(citations) + 1) // 2
        columns = (citations[:middle], citations[middle:])
        rows = [[column[i] if i < len(column) else "" for column in columns] for i in range(middle)]
        self.table(["References", "References (continued)"], rows, [.5, .5])

    def render(self, destination):
        self.cover()
        self.main_report()
        self.history()
        self.banking()
        self.rule_appendix()
        self.calculations()
        self.bibliography()
        doc = PDFDocument(str(destination), pagesize=A4, leftMargin=44, rightMargin=44,
                          topMargin=49, bottomMargin=47, title="BBCA Fundamental Analysis - FY2025",
                          author="Alcazar", subject="FY2025 results, history, catalog rules and calculations")

        def page(canvas, _doc):
            canvas.setStrokeColor(LINE)
            canvas.line(44, A4[1] - 35, A4[0] - 44, A4[1] - 35)
            canvas.line(44, 35, A4[0] - 44, 35)
            canvas.setFont(FONT, 8)
            canvas.setFillColor(MUTED)
            canvas.drawString(44, A4[1] - 28, "BBCA  /  FUNDAMENTAL ANALYSIS")
            canvas.drawString(44, 23, "FY2025  /  ANALYSIS AND SOURCE EVIDENCE")
            canvas.drawRightString(A4[0] - 44, 23, str(canvas.getPageNumber()))

        doc.build(self.story, onFirstPage=page, onLaterPages=page)


def main():
    cli = argparse.ArgumentParser(description=__doc__)
    cli.add_argument("report", type=Path, help="BBCA analysis JSON")
    cli.add_argument("output", type=Path, help="destination PDF")
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
