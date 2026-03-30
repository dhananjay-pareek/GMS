package main

import (
	"context"
	"encoding/csv"
	"os"

	"github.com/dhananjay-pareek/google-maps-scraper/internal/scrapemate"

	"github.com/dhananjay-pareek/google-maps-scraper/internal/scrapemate/adapters/writers/csvwriter"
	"github.com/dhananjay-pareek/google-maps-scraper/internal/scrapemate/quotestoscrapelogin/quotes"
	"github.com/dhananjay-pareek/google-maps-scraper/internal/scrapemate/scrapemateapp"
)

func main() {
	if err := run(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
		return
	}
	os.Exit(0)
}

func run() error {
	csvWriter := csvwriter.NewCsvWriter(csv.NewWriter(os.Stdout))

	writers := []scrapemate.ResultWriter{
		csvWriter,
	}

	cfg, err := scrapemateapp.NewConfig(writers,
		scrapemateapp.WithInitJob(quotes.NewLoginCRSFToken()),
	)
	if err != nil {
		return err
	}

	app, err := scrapemateapp.NewScrapeMateApp(cfg)
	if err != nil {
		return err
	}

	seedJobs := []scrapemate.IJob{
		quotes.NewQuoteCollectJob("https://quotes.toscrape.com/"),
	}
	return app.Start(context.Background(), seedJobs...)
}
