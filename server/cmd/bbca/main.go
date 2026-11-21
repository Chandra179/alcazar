package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Chandra179/lux/server/internal/financials"
	"github.com/Chandra179/lux/server/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bbca:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("expected import, catalog, review, approve, or analysis subcommand")
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dataDir := flags.String("data", "./data", "directory containing source reports")
	extractedDir := flags.String("extracted", "", "page text output directory (default: data/extracted)")
	dbPath := flags.String("db", defaultDBPath(), "SQLite database path")
	catalogPath := flags.String("catalog", defaultCatalogPath(), "fundamental-analysis.yaml path")
	outPath := flags.String("out", filepath.Join("data", "review", "bbca-review.csv"), "review CSV output path")
	filePath := flags.String("file", "", "review CSV to apply")
	reviewPeriod := flags.String("period", "", "optional period of pending facts to export")
	reviewDocument := flags.String("document", "", "optional source path relative to data for review export")
	ocrLanguage := flags.String("ocr-language", "eng+ind", "Tesseract language set")
	minPageChars := flags.Int("min-page-chars", 60, "OCR pages with fewer non-space characters")
	skipOCR := flags.Bool("skip-ocr", false, "use PDF text without OCR for documents that have a verified text layer")
	force := flags.Bool("force", false, "rebuild facts from cached pages and extract any missing pages")
	analysisFormat := flags.String("format", "markdown", "analysis output format (markdown, json, or facts-hash)")
	view := flags.String("view", "brief", "analysis report view (brief, evidence, or yearly)")
	evidencePath := flags.String("evidence", filepath.Join("data", "analysis", "bbca-fy2025.yaml"), "analysis evidence file")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}

	ctx := context.Background()
	if command == "analysis" {
		return runAnalysis(ctx, *dbPath, *catalogPath, *evidencePath, *analysisFormat, *view)
	}
	db, err := store.NewSQLite(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := store.EnsureBBCAFinancialSchema(ctx, db); err != nil {
		return err
	}
	switch command {
	case "catalog":
		return financials.Initialize(ctx, db, *catalogPath)
	case "import":
		summary, err := financials.Import(ctx, db, financials.ImportOptions{
			DataDir: *dataDir, CatalogPath: *catalogPath,
			ExtractedDir: *extractedDir, OCRLanguage: *ocrLanguage,
			MinPageChars: *minPageChars, Force: *force,
			SkipOCR: *skipOCR,
		})
		if err != nil {
			return err
		}
		fmt.Printf("documents=%d pages=%d facts=%d skipped=%d\n", summary.Documents, summary.Pages, summary.Facts, summary.Skipped)
		return nil
	case "review":
		if err := financials.ExportReviewCSV(ctx, db, *outPath, financials.ReviewFilter{Period: *reviewPeriod, Document: *reviewDocument}); err != nil {
			return err
		}
		fmt.Printf("review CSV: %s\n", *outPath)
		return nil
	case "approve":
		if strings.TrimSpace(*filePath) == "" {
			return errors.New("approve requires --file path/to/review.csv")
		}
		count, err := financials.ApplyReviewCSV(ctx, db, *filePath)
		if err != nil {
			return err
		}
		fmt.Printf("reviewed facts=%d\n", count)
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (use import, catalog, review, approve, or analysis)", command)
	}
}

func runAnalysis(ctx context.Context, dbPath, catalogPath, evidencePath, format, view string) error {
	if format != "markdown" && format != "json" && format != "facts-hash" {
		return fmt.Errorf("invalid analysis format %q (use markdown, json, or facts-hash)", format)
	}
	if view != "brief" && view != "evidence" && view != "yearly" {
		return fmt.Errorf("invalid analysis view %q (use brief, evidence, or yearly)", view)
	}
	path, err := filepath.Abs(dbPath)
	if err != nil {
		return err
	}
	dsn := url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: "mode=ro"}
	db, err := store.NewSQLite(ctx, dsn.String())
	if err != nil {
		return fmt.Errorf("open analysis database %q; see data/README.md for setup: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()
	if format == "facts-hash" {
		hash, err := financials.AnalysisFactsHash(ctx, db)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, hash)
		return err
	}
	report, err := financials.BuildAnalysis(ctx, db, catalogPath, evidencePath)
	if err != nil {
		return err
	}
	return report.WriteView(os.Stdout, format, view)
}

func defaultCatalogPath() string {
	if value := strings.TrimSpace(os.Getenv("BBCA_CATALOG")); value != "" {
		return value
	}
	return "fundamental-analysis.yaml"
}

func defaultDBPath() string {
	if value := strings.TrimSpace(os.Getenv("SQLITE_DSN")); value != "" {
		return value
	}
	return "lux.db"
}
