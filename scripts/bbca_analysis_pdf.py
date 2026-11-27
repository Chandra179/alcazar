#!/usr/bin/env python3
"""Render an HTML report generated from standard-1.html.tmpl to PDF via headless Chrome."""

import argparse
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


def find_browser():
    """Return the first available headless Chromium-based browser binary."""
    for name in ("google-chrome", "chromium", "chromium-browser"):
        path = shutil.which(name)
        if path:
            return path
    sys.exit("PDF generation requires google-chrome, chromium, or chromium-browser.")


def render_pdf(html_path: Path, output_path: Path) -> None:
    """Print the HTML file to PDF atomically using headless Chrome."""
    browser = find_browser()
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".bbca-pdf-", dir=output_path.parent) as directory:
        draft = Path(directory) / "report.pdf"
        cmd = [
            browser,
            "--headless=new",
            "--disable-gpu",
            "--no-sandbox",
            "--no-pdf-header-footer",
            f"--print-to-pdf={draft.resolve()}",
            html_path.resolve().as_uri(),
        ]
        subprocess.run(cmd, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True)
        os.replace(draft, output_path)


def main():
    cli = argparse.ArgumentParser(description=__doc__)
    cli.add_argument("html", type=Path, help="rendered report HTML")
    cli.add_argument("output", type=Path, help="destination PDF")
    args = cli.parse_args()
    render_pdf(args.html, args.output)
    print(args.output)


if __name__ == "__main__":
    main()
