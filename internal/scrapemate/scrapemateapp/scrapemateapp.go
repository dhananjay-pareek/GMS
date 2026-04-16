package scrapemateapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/gosom/google-maps-scraper/internal/scrapemate"
	"golang.org/x/sync/errgroup"

	"github.com/gosom/google-maps-scraper/internal/scrapemate/adapters/cache/filecache"
	"github.com/gosom/google-maps-scraper/internal/scrapemate/adapters/cache/leveldbcache"
	fetcher "github.com/gosom/google-maps-scraper/internal/scrapemate/adapters/fetchers/nethttp"
	"github.com/gosom/google-maps-scraper/internal/scrapemate/adapters/fetchers/stealth"
	parser "github.com/gosom/google-maps-scraper/internal/scrapemate/adapters/parsers/goqueryparser"
	memprovider "github.com/gosom/google-maps-scraper/internal/scrapemate/adapters/providers/memory"
	"github.com/gosom/google-maps-scraper/internal/scrapemate/adapters/proxy"
)

type ScrapemateApp struct {
	cfg *Config

	ctx    context.Context
	cancel context.CancelCauseFunc

	provider scrapemate.JobProvider
	cacher   scrapemate.Cacher
}

// NewScrapemateApp creates a new ScrapemateApp.
func NewScrapeMateApp(cfg *Config) (*ScrapemateApp, error) {
	app := ScrapemateApp{
		cfg: cfg,
	}

	return &app, nil
}

// Start starts the app.
func (app *ScrapemateApp) Start(ctx context.Context, seedJobs ...scrapemate.IJob) error {
	g, ctx := errgroup.WithContext(ctx)
	ctx, cancel := context.WithCancelCause(ctx)

	defer cancel(errors.New("closing app"))

	mate, err := app.getMate(ctx)
	if err != nil {
		return err
	}

	defer app.Close()
	defer mate.Close()

	// Create per-writer channels for fan-out (broadcast to all writers)
	writerChans := make([]chan scrapemate.Result, len(app.cfg.Writers))
	for i := range app.cfg.Writers {
		writerChans[i] = make(chan scrapemate.Result, 100)
	}

	// Start writers with their individual channels
	for i := range app.cfg.Writers {
		writer := app.cfg.Writers[i]
		ch := writerChans[i]

		g.Go(func() error {
			if err := writer.Run(ctx, ch); err != nil {
				cancel(err)
				return err
			}

			return nil
		})
	}

	// Fan-out: broadcast results to all writer channels
	g.Go(func() error {
		defer func() {
			for _, ch := range writerChans {
				close(ch)
			}
		}()

		for result := range mate.Results() {
			for _, ch := range writerChans {
				select {
				case ch <- result:
				case <-ctx.Done():
					// During shutdown, try to push but don't block indefinitely
					select {
					case ch <- result:
					default:
						// If channel is full, we must drop to avoid deadlock
					}
				}
			}
		}
		return nil
	})

	g.Go(func() error {
		return mate.Start()
	})

	g.Go(func() error {
		for i := range seedJobs {
			if err := app.provider.Push(ctx, seedJobs[i]); err != nil {
				return err
			}
		}

		return nil
	})

	return g.Wait()
}

// Close closes the app.
func (app *ScrapemateApp) Close() error {
	if app.cacher != nil {
		app.cacher.Close()
	}

	return nil
}

func (app *ScrapemateApp) getMate(ctx context.Context) (*scrapemate.ScrapeMate, error) {
	var err error

	app.provider, err = app.getProvider()
	if err != nil {
		return nil, err
	}

	fetcherInstance, err := app.getFetcher()
	if err != nil {
		return nil, err
	}

	app.cacher, err = app.getCacher()
	if err != nil {
		return nil, err
	}

	params := []func(*scrapemate.ScrapeMate) error{
		scrapemate.WithContext(ctx, app.cancel),
		scrapemate.WithJobProvider(app.provider),
		scrapemate.WithHTTPFetcher(fetcherInstance),
		scrapemate.WithHTMLParser(parser.New()),
		scrapemate.WithConcurrency(app.cfg.Concurrency),
		scrapemate.WithExitBecauseOfInactivity(app.cfg.ExitOnInactivityDuration),
	}

	if app.cacher != nil {
		params = append(params, scrapemate.WithCache(app.cacher))
	}

	if app.cfg.InitJob != nil {
		params = append(params, scrapemate.WithInitJob(app.cfg.InitJob))
	}

	return scrapemate.New(params...)
}

func (app *ScrapemateApp) getCacher() (scrapemate.Cacher, error) {
	var (
		cacher scrapemate.Cacher
		err    error
	)

	switch app.cfg.CacheType {
	case "file":
		cacher, err = filecache.NewFileCache(app.cfg.CachePath)
	case "leveldb":
		cacher, err = leveldbcache.NewLevelDBCache(app.cfg.CachePath)
	}

	return cacher, err
}

func (app *ScrapemateApp) getFetcher() (scrapemate.HTTPFetcher, error) {
	var (
		httpFetcher scrapemate.HTTPFetcher
		err         error
		rotator     scrapemate.ProxyRotator
	)

	if len(app.cfg.Proxies) > 0 {
		rotator = proxy.New(app.cfg.Proxies)
	}

	const timeout = 10 * time.Second

	switch app.cfg.UseJS {
	case true:
		httpFetcher, err = app.getJSFetcher(rotator)
		if err != nil {
			return nil, err
		}
	default:
		if app.cfg.UseStealth {
			httpFetcher = stealth.New(
				app.cfg.StealthBrowser,
				rotator,
			)
		} else {
			cookieJar, err := cookiejar.New(nil)
			if err != nil {
				return nil, err
			}

			transport := &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				MaxConnsPerHost:     20,
				IdleConnTimeout:     90 * time.Second,
			}

			netClient := &http.Client{
				Timeout:   timeout,
				Jar:       cookieJar,
				Transport: transport,
			}

			if rotator != nil {
				netClient.Transport = rotator
			}

			httpFetcher = fetcher.New(netClient)
		}
	}

	return httpFetcher, nil
}

//nolint:unparam // this function returns always nil error
func (app *ScrapemateApp) getProvider() (scrapemate.JobProvider, error) {
	var provider scrapemate.JobProvider

	switch app.cfg.Provider {
	case nil:
		provider = memprovider.New()
	default:
		provider = app.cfg.Provider
	}

	return provider, nil
}
