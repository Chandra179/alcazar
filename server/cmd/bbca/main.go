package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
		return errors.New("expected import, review, or approve subcommand")
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dataDir := flags.String("data", "./data", "directory containing source reports")
	dbPath := flags.String("db", "lux.db", "SQLite database path")
	catalogPath := flags.String("catalog", defaultCatalogPath(), "fundamental-analysis.yaml path")
	outPath := flags.String("out", filepath.Join("data", "review", "bbca-review.csv"), "review CSV output path")
	filePath := flags.String("file", "", "review CSV to apply")
	ocrLanguage := flags.String("ocr-language", "eng+ind", "Tesseract language set")
	minPageChars := flags.Int("min-page-chars", 60, "OCR pages with fewer non-space characters")
	force := flags.Bool("force", false, "re-extract processed documents")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}

	ctx := context.Background()
	db, err := store.NewSQLite(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := financials.Initialize(ctx, db, *catalogPath); err != nil {
		return err
	}
	switch command {
	case "import":
		summary, err := financials.Import(ctx, db, financials.ImportOptions{
			DataDir: *dataDir, CatalogPath: *catalogPath,
			ExtractedDir: filepath.Join(*dataDir, "extracted"), OCRLanguage: *ocrLanguage,
			MinPageChars: *minPageChars, Force: *force,
		})
		if err != nil {
			return err
		}
		fmt.Printf("documents=%d pages=%d facts=%d skipped=%d\n", summary.Documents, summary.Pages, summary.Facts, summary.Skipped)
		return nil
	case "review":
		if err := financials.ExportReviewCSV(ctx, db, *outPath); err != nil {
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
		return fmt.Errorf("unknown subcommand %q (use import, review, or approve)", command)
	}
}

func defaultCatalogPath() string {
	if value := strings.TrimSpace(os.Getenv("BBCA_CATALOG")); value != "" {
		return value
	}
	return filepath.Join("..", "atlas", "business", "fundamental-analysis.yaml")
}
