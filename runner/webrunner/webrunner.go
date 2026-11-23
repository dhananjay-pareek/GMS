package webrunner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gosom/google-maps-scraper/deduper"
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/google-maps-scraper/localdbwriter"
	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/supabasewriter"
	"github.com/gosom/google-maps-scraper/tlmt"
	"github.com/gosom/google-maps-scraper/web"
	"github.com/gosom/google-maps-scraper/web/jsonrepo"
	"github.com/gosom/google-maps-scraper/web/sqlite"
	"github.com/gosom/scrapemate"
	"github.com/gosom/scrapemate/scrapemateapp"
	"golang.org/x/sync/errgroup"
)

type webrunner struct {
	srv *web.Server
	svc *web.Service
	cfg *runner.Config
}

func New(cfg *runner.Config) (runner.Runner, error) {
	if cfg.DataFolder == "" {
		return nil, fmt.Errorf("data folder is required")
	}

	if err := os.MkdirAll(cfg.DataFolder, os.ModePerm); err != nil {
		return nil, err
	}

	var repo web.JobRepository
	var err error

	if cfg.DBType == "json" {
		dbpath := filepath.Join(cfg.DataFolder, "jobs.json")
		repo, err = jsonrepo.New(dbpath)
	} else {
		dbpath := filepath.Join(cfg.DataFolder, "jobs.db")
		repo, err = sqlite.New(dbpath)
	}

	if err != nil {
		return nil, err
	}

	svc := web.NewService(repo, cfg.DataFolder)

	srv, err := web.New(
		svc,
		cfg.Addr,
		cfg.LeadsManagerURL,
		cfg.LLMProvider,
		cfg.LLMAPIKey,
		cfg.LLMModel,
		cfg.OllamaURL,
	)
	if err != nil {
		return nil, err
	}

	ans := webrunner{
		srv: srv,
		svc: svc,
		cfg: cfg,
	}

	return &ans, nil
}

func (w *webrunner) Run(ctx context.Context) error {
	egroup, ctx := errgroup.WithContext(ctx)

	egroup.Go(func() error {
		return w.work(ctx)
	})

	egroup.Go(func() error {
		return w.srv.Start(ctx)
	})

	// Auto-open browser after server is ready
	go func() {
		url := "http://localhost" + w.cfg.Addr

		// Wait for server to be ready with longer timeout
		readyTimeout := time.NewTimer(10 * time.Second)
		defer readyTimeout.Stop()

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				resp, err := http.Get(url + "/health")
				if err == nil {
					resp.Body.Close()
					// Server is ready, try to open browser
					log.Printf("[BROWSER] Opening %s", url)
					if err := openBrowser(url); err != nil {
						log.Printf("[BROWSER] Failed to open browser: %v", err)
						log.Printf("[BROWSER] Please manually open: %s", url)
					} else {
						log.Printf("[BROWSER] Browser opened successfully")
					}
					return
				}
			case <-readyTimeout.C:
				log.Printf("[BROWSER] Server not ready after 10 seconds, skipping auto-open")
				log.Printf("[BROWSER] Please manually open: %s", url)
				return
			}
		}
	}()

	return egroup.Wait()
}

// openBrowser opens the specified URL in app mode window
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		edgePath := findEdgePath()
		if edgePath != "" {
			log.Printf("[BROWSER] Using Edge: %s", edgePath)
			cmd = exec.Command(edgePath, "--app="+url, "--window-size=1200,800")
		} else {
			// Try Chrome as fallback
			chromePath := findChromePath()
			if chromePath != "" {
				log.Printf("[BROWSER] Using Chrome: %s", chromePath)
				cmd = exec.Command(chromePath, "--app="+url, "--window-size=1200,800")
			} else {
				log.Printf("[BROWSER] No browser found, using rundll32")
				cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
			}
		}
	case "darwin":
		chromePath := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := os.Stat(chromePath); err == nil {
			log.Printf("[BROWSER] Using Chrome: %s", chromePath)
			cmd = exec.Command(chromePath, "--app="+url, "--window-size=1200,800")
		} else {
			log.Printf("[BROWSER] Using default open command")
			cmd = exec.Command("open", url)
		}
	default:
		for _, browser := range []string{"google-chrome", "chromium-browser", "chromium"} {
			if path, err := exec.LookPath(browser); err == nil {
				log.Printf("[BROWSER] Using %s: %s", browser, path)
				cmd = exec.Command(path, "--app="+url, "--window-size=1200,800")
				break
			}
		}
		if cmd == nil {
			log.Printf("[BROWSER] Using xdg-open")
			cmd = exec.Command("xdg-open", url)
		}
	}

	if cmd == nil {
		return fmt.Errorf("no browser command created")
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start browser: %w", err)
	}

	return nil
}

// findEdgePath finds Microsoft Edge executable on Windows
func findEdgePath() string {
	paths := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Microsoft", "Edge", "Application", "msedge.exe"),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// findChromePath finds Google Chrome executable on Windows
func findChromePath() string {
	paths := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

func (w *webrunner) Close(context.Context) error {
	return nil
}

func (w *webrunner) work(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			jobs, err := w.svc.SelectPending(ctx)
			if err != nil {
				return err
			}

			for i := range jobs {
				select {
				case <-ctx.Done():
					return nil
				default:
					t0 := time.Now().UTC()
					if err := w.scrapeJob(ctx, &jobs[i]); err != nil {
						params := map[string]any{
							"job_count": len(jobs[i].Data.Keywords),
							"duration":  time.Now().UTC().Sub(t0).String(),
							"error":     err.Error(),
						}

						evt := tlmt.NewEvent("web_runner", params)

						_ = runner.Telemetry().Send(ctx, evt)

						log.Printf("error scraping job %s: %v", jobs[i].ID, err)
					} else {
						params := map[string]any{
							"job_count": len(jobs[i].Data.Keywords),
							"duration":  time.Now().UTC().Sub(t0).String(),
						}

						_ = runner.Telemetry().Send(ctx, tlmt.NewEvent("web_runner", params))

						log.Printf("job %s scraped successfully", jobs[i].ID)
					}
				}
			}
		}
	}
}

func (w *webrunner) scrapeJob(ctx context.Context, job *web.Job) error {
	persistJob := func(errMsg string) error {
		persistCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if errMsg != "" {
			job.Error = errMsg
			log.Printf("[JOB %s] ERROR: %s | Status: %s", job.ID, errMsg, job.Status)
		}
		job.CompletedAt = time.Now().UTC()

		if err := w.svc.Update(persistCtx, job); err != nil {
			return fmt.Errorf("persist job %s status %q: %w", job.ID, job.Status, err)
		}

		return nil
	}

	log.Printf("[JOB %s] Starting scrape job: %q (keywords: %d, lang: %s)",
		job.ID, job.Name, len(job.Data.Keywords), job.Data.Lang)

	job.Status = web.StatusWorking
	if err := persistJob(""); err != nil {
		return err
	}

	if len(job.Data.Keywords) == 0 {
		job.Status = web.StatusFailed
		errMsg := "no keywords provided"
		log.Printf("[JOB %s] FAILURE: %s", job.ID, errMsg)
		return persistJob(errMsg)
	}

	mate, err := w.setupMate(ctx, job)
	if err != nil {
		job.Status = web.StatusFailed
		errMsg := fmt.Sprintf("setup failed: %v", err)
		if err2 := persistJob(errMsg); err2 != nil {
			log.Printf("[JOB %s] failed to update job status: %v", job.ID, err2)
		}
		return err
	}

	defer mate.Close()

	var coords string
	if job.Data.Lat != "" && job.Data.Lon != "" {
		coords = job.Data.Lat + "," + job.Data.Lon
	}

	dedup := deduper.New()
	exitMonitor := exiter.New()

	log.Printf("[JOB %s] Creating seed jobs with depth=%d, coords=%q, zoom=%d",
		job.ID, job.Data.Depth, coords, job.Data.Zoom)

	seedJobs, err := runner.CreateSeedJobs(
		job.Data.FastMode,
		job.Data.Lang,
		strings.NewReader(strings.Join(job.Data.Keywords, "\n")),
		job.Data.Depth,
		job.Data.Email,
		coords,
		job.Data.Zoom,
		func() float64 {
			if job.Data.Radius <= 0 {
				return 10000 // 10 km
			}

			return float64(job.Data.Radius)
		}(),
		dedup,
		exitMonitor,
		w.cfg.ExtraReviews,
	)
	if err != nil {
		job.Status = web.StatusFailed
		errMsg := fmt.Sprintf("create seed jobs failed: %v", err)
		if err2 := persistJob(errMsg); err2 != nil {
			log.Printf("[JOB %s] failed to update job status: %v", job.ID, err2)
		}
		return err
	}

	log.Printf("[JOB %s] Created %d seed jobs", job.ID, len(seedJobs))

	if len(seedJobs) > 0 {
		exitMonitor.SetSeedCount(len(seedJobs))

		allowedSeconds := max(60, len(seedJobs)*10*job.Data.Depth/50+120)

		if job.Data.MaxTime > 0 {
			if job.Data.MaxTime.Seconds() < 180 {
				allowedSeconds = 180
			} else {
				allowedSeconds = int(job.Data.MaxTime.Seconds())
			}
		}

		log.Printf("[JOB %s] Running scrape with %d seed jobs, max %d seconds, concurrency=%d",
			job.ID, len(seedJobs), allowedSeconds, w.cfg.Concurrency)

		mateCtx, cancel := context.WithTimeout(ctx, time.Duration(allowedSeconds)*time.Second)
		defer cancel()

		exitMonitor.SetCancelFunc(cancel)

		go exitMonitor.Run(mateCtx)

		log.Printf("[JOB %s] Starting scrapemate...", job.ID)
		startErr := mate.Start(mateCtx, seedJobs...)
		wasInterrupted := errors.Is(startErr, context.Canceled) || errors.Is(startErr, context.DeadlineExceeded) ||
			mateCtx.Err() != nil || ctx.Err() != nil
		if startErr != nil && !errors.Is(startErr, context.DeadlineExceeded) && !errors.Is(startErr, context.Canceled) {
			cancel()

			job.Status = web.StatusFailed
			errMsg := fmt.Sprintf("scraping failed: %v", startErr)
			if err2 := persistJob(errMsg); err2 != nil {
				log.Printf("[JOB %s] failed to update job status: %v", job.ID, err2)
			}

			return startErr
		}

		if wasInterrupted {
			job.Status = web.StatusFailed
			job.Error = "job aborted: shutdown"
			if errors.Is(startErr, context.DeadlineExceeded) || errors.Is(mateCtx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				job.Error = "job aborted: timeout"
			}
			log.Printf("[JOB %s] Scraping interrupted: %s", job.ID, job.Error)
		} else {
			log.Printf("[JOB %s] Scraping completed successfully", job.ID)
		}

		// DO NOT cancel context yet - writers need time to flush batched results
		// Instead, let Close() trigger proper writer shutdown
		log.Printf("[JOB %s] Closing scrapemate and waiting for writers to flush...", job.ID)
		mate.Close()

		// Give writers a grace period to complete (up to 15 seconds)
		// They will auto-exit when input channel closes
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer flushCancel()

		// Wait for context to be truly done (writers finishing)
		select {
		case <-mateCtx.Done():
			log.Printf("[JOB %s] Scrapemate context done", job.ID)
		case <-flushCtx.Done():
			log.Printf("[JOB %s] Writer flush timeout - continuing anyway", job.ID)
		case <-time.After(3 * time.Second):
			log.Printf("[JOB %s] Giving writers extra time to flush", job.ID)
		}

		cancel() // Now safe to cancel

		if wasInterrupted {
			if err := persistJob(job.Error); err != nil {
				log.Printf("[JOB %s] failed to update job status: %v", job.ID, err)
			}

			if errors.Is(startErr, context.DeadlineExceeded) || errors.Is(mateCtx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return context.DeadlineExceeded
			}

			return context.Canceled
		}
	} else {
		log.Printf("[JOB %s] No seed jobs were created", job.ID)
	}


	job.Status = web.StatusOK
	log.Printf("[JOB %s] Job completed successfully", job.ID)
	return persistJob("")
}

func (w *webrunner) setupMate(_ context.Context, job *web.Job) (*scrapemateapp.ScrapemateApp, error) {
	opts := []func(*scrapemateapp.Config) error{
		scrapemateapp.WithConcurrency(w.cfg.Concurrency),
		scrapemateapp.WithExitOnInactivity(w.cfg.ExitOnInactivityDuration),
	}

	if !job.Data.FastMode {
		opts = append(opts,
			scrapemateapp.WithJS(scrapemateapp.DisableImages()),
		)
	} else {
		opts = append(opts,
			scrapemateapp.WithStealth("firefox"),
		)
	}

	hasProxy := false

	if len(w.cfg.Proxies) > 0 {
		opts = append(opts, scrapemateapp.WithProxies(w.cfg.Proxies))
		hasProxy = true
	} else if len(job.Data.Proxies) > 0 {
		opts = append(opts,
			scrapemateapp.WithProxies(job.Data.Proxies),
		)
		hasProxy = true
	}

	if !w.cfg.DisablePageReuse {
		opts = append(opts,
			scrapemateapp.WithPageReuseLimit(2),
			scrapemateapp.WithBrowserReuseLimit(200),
		)
	}

	log.Printf("job %s has proxy: %v", job.ID, hasProxy)

	var writers []scrapemate.ResultWriter
	csvPath := filepath.Join(w.cfg.DataFolder, job.ID+".csv")
	csvWriter, err := newCSVJobWriter(csvPath)
	if err != nil {
		return nil, fmt.Errorf("create csv writer: %w", err)
	}
	writers = append(writers, csvWriter)

	disableLocal := os.Getenv("DISABLE_LOCAL_DB") == "true"
	if !disableLocal {
		localWriter, err := localdbwriter.New(context.Background(), w.cfg.LeadsDBPath)
		if err != nil {
			return nil, fmt.Errorf("create local leads db writer: %w", err)
		}
		writers = append(writers, localWriter)
	} else {
		log.Println("Local DB writer disabled via DISABLE_LOCAL_DB")
	}

	// If DATABASE_URL is set, also write to Supabase
	if w.cfg.Dsn != "" {
		sbWriter, sbErr := supabasewriter.New(w.cfg.Dsn)
		if sbErr != nil {
			log.Printf("WARNING: supabase writer init failed: %v", sbErr)
		} else {
			log.Println("Supabase writer enabled")
			writers = append(writers, sbWriter)
		}
	}

	if len(writers) == 0 {
		return nil, fmt.Errorf("no result writers configured")
	}

	var resultWriter scrapemate.ResultWriter
	if len(writers) == 1 {
		resultWriter = writers[0]
	} else {
		resultWriter = runner.NewMultiWriter(writers...)
	}

	matecfg, err := scrapemateapp.NewConfig(
		[]scrapemate.ResultWriter{resultWriter},
		opts...,
	)
	if err != nil {
		return nil, err
	}

	return scrapemateapp.NewScrapeMateApp(matecfg)
}
