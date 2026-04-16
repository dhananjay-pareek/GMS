package filerunner

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/gosom/google-maps-scraper/deduper"
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/google-maps-scraper/localdbwriter"
	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/supabasewriter"
	"github.com/gosom/google-maps-scraper/tlmt"
	"github.com/gosom/scrapemate"
	"github.com/gosom/scrapemate/scrapemateapp"
)

type fileRunner struct {
	cfg     *runner.Config
	input   io.Reader
	writers []scrapemate.ResultWriter
	app     *scrapemateapp.ScrapemateApp
}

func New(cfg *runner.Config) (runner.Runner, error) {
	if cfg.RunMode != runner.RunModeFile {
		return nil, fmt.Errorf("%w: %d", runner.ErrInvalidRunMode, cfg.RunMode)
	}

	ans := &fileRunner{
		cfg: cfg,
	}

	if err := ans.setInput(); err != nil {
		return nil, err
	}

	if err := ans.setWriters(); err != nil {
		return nil, err
	}

	if err := ans.setApp(); err != nil {
		return nil, err
	}

	return ans, nil
}

func (r *fileRunner) Run(ctx context.Context) (err error) {
	var seedJobs []scrapemate.IJob

	t0 := time.Now().UTC()

	defer func() {
		elapsed := time.Now().UTC().Sub(t0)
		params := map[string]any{
			"job_count": len(seedJobs),
			"duration":  elapsed.String(),
		}

		if err != nil {
			params["error"] = err.Error()
		}

		evt := tlmt.NewEvent("file_runner", params)

		_ = runner.Telemetry().Send(ctx, evt)
	}()

	dedup := deduper.New()
	exitMonitor := exiter.New()

	seedJobs, err = runner.CreateSeedJobs(
		r.cfg.FastMode,
		r.cfg.LangCode,
		r.input,
		r.cfg.MaxDepth,
		r.cfg.Email,
		r.cfg.GeoCoordinates,
		r.cfg.Zoom,
		r.cfg.Radius,
		dedup,
		exitMonitor,
		r.cfg.ExtraReviews,
	)
	if err != nil {
		return err
	}

	exitMonitor.SetSeedCount(len(seedJobs))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	exitMonitor.SetCancelFunc(cancel)

	go exitMonitor.Run(ctx)

	err = r.app.Start(ctx, seedJobs...)

	return err
}

func (r *fileRunner) Close(context.Context) error {
	if r.app != nil {
		return r.app.Close()
	}

	if r.input != nil {
		if closer, ok := r.input.(io.Closer); ok {
			return closer.Close()
		}
	}

	return nil
}

func (r *fileRunner) setInput() error {
	switch r.cfg.InputFile {
	case "stdin":
		r.input = os.Stdin
	default:
		f, err := os.Open(r.cfg.InputFile)
		if err != nil {
			return err
		}

		r.input = f
	}

	return nil
}

func (r *fileRunner) setWriters() error {
	var writers []scrapemate.ResultWriter

	disableLocal := os.Getenv("DISABLE_LOCAL_DB") == "true"
	var localWriter scrapemate.ResultWriter

	if !disableLocal {
		lWriter, err := localdbwriter.New(context.Background(), r.cfg.LeadsDBPath)
		if err != nil {
			return fmt.Errorf("create local leads db writer: %w", err)
		}
		localWriter = lWriter
		writers = append(writers, localWriter)
	} else {
		log.Println("Local DB writer disabled via DISABLE_LOCAL_DB")
	}

	// If DATABASE_URL is set, also write to Supabase
	if r.cfg.Dsn != "" {
		sbWriter, sbErr := supabasewriter.New(r.cfg.Dsn)
		if sbErr != nil {
			log.Printf("WARNING: supabase writer init failed: %v", sbErr)
		} else {
			log.Println("Supabase writer enabled")
			writers = append(writers, sbWriter)
		}
	}

	if len(writers) == 1 {
		r.writers = writers
	} else {
		r.writers = []scrapemate.ResultWriter{runner.NewMultiWriter(writers...)}
	}

	return nil
}

func (r *fileRunner) setApp() error {
	opts := []func(*scrapemateapp.Config) error{
		// scrapemateapp.WithCache("leveldb", "cache"),
		scrapemateapp.WithConcurrency(r.cfg.Concurrency),
		scrapemateapp.WithExitOnInactivity(r.cfg.ExitOnInactivityDuration),
	}

	if len(r.cfg.Proxies) > 0 {
		opts = append(opts,
			scrapemateapp.WithProxies(r.cfg.Proxies),
		)
	}

	if !r.cfg.FastMode {
		if r.cfg.Debug {
			opts = append(opts, scrapemateapp.WithJS(
				scrapemateapp.Headfull(),
				scrapemateapp.DisableImages(),
			))
		} else {
			opts = append(opts, scrapemateapp.WithJS(scrapemateapp.DisableImages()))
		}
	} else {
		opts = append(opts, scrapemateapp.WithStealth("firefox"))
	}

	if !r.cfg.DisablePageReuse {
		opts = append(opts,
			scrapemateapp.WithPageReuseLimit(2),
			scrapemateapp.WithBrowserReuseLimit(200),
		)
	}

	matecfg, err := scrapemateapp.NewConfig(
		r.writers,
		opts...,
	)
	if err != nil {
		return err
	}

	r.app, err = scrapemateapp.NewScrapeMateApp(matecfg)
	if err != nil {
		return err
	}

	return nil
}
