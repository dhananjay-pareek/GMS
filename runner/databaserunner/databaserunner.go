package databaserunner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	// postgres driver
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/gosom/google-maps-scraper/postgres"
	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/tlmt"
	"github.com/gosom/scrapemate"
	"github.com/gosom/scrapemate/scrapemateapp"
)

type dbrunner struct {
	cfg      *runner.Config
	provider scrapemate.JobProvider
	produce  bool
	app      *scrapemateapp.ScrapemateApp
	conn     *sql.DB
}

func New(cfg *runner.Config) (runner.Runner, error) {
	if cfg.RunMode != runner.RunModeDatabase && cfg.RunMode != runner.RunModeDatabaseProduce {
		return nil, fmt.Errorf("%w: %d", runner.ErrInvalidRunMode, cfg.RunMode)
	}

	conn, err := openPsqlConn(cfg.Dsn)
	if err != nil {
		return nil, err
	}

	ans := dbrunner{
		cfg:      cfg,
		provider: postgres.NewProvider(conn),
		produce:  cfg.ProduceOnly,
		conn:     conn,
	}

	if ans.produce {
		return &ans, nil
	}

	if reclaimed, reclaimErr := finalizeQueuedJobs(conn, "startup reset"); reclaimErr != nil {
		return nil, reclaimErr
	} else if reclaimed > 0 {
		log.Printf("databaserunner: finalized %d stale queued jobs on startup", reclaimed)
	}

	psqlWriter := postgres.NewResultWriter(conn)

	writers := []scrapemate.ResultWriter{
		psqlWriter,
	}

	opts := []func(*scrapemateapp.Config) error{
		// scrapemateapp.WithCache("leveldb", "cache"),
		scrapemateapp.WithConcurrency(cfg.Concurrency),
		scrapemateapp.WithProvider(ans.provider),
		scrapemateapp.WithExitOnInactivity(cfg.ExitOnInactivityDuration),
	}

	if len(cfg.Proxies) > 0 {
		opts = append(opts,
			scrapemateapp.WithProxies(cfg.Proxies),
		)
	}

	if !cfg.FastMode {
		if cfg.Debug {
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

	if !cfg.DisablePageReuse {
		opts = append(opts,
			scrapemateapp.WithPageReuseLimit(2),
			scrapemateapp.WithPageReuseLimit(200),
		)
	}

	matecfg, err := scrapemateapp.NewConfig(
		writers,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	ans.app, err = scrapemateapp.NewScrapeMateApp(matecfg)
	if err != nil {
		return nil, err
	}

	return &ans, nil
}

func (d *dbrunner) Run(ctx context.Context) error {
	_ = runner.Telemetry().Send(ctx, tlmt.NewEvent("databaserunner.Run", nil))

	if d.produce {
		return d.produceSeedJobs(ctx)
	}

	return d.app.Start(ctx)
}


func (d *dbrunner) Close(context.Context) error {
	var closeErr error

	if d.app != nil {
		if err := d.app.Close(); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}

	if !d.produce && d.conn != nil {
		if finalized, err := finalizeQueuedJobs(d.conn, "shutdown"); err != nil {
			closeErr = errors.Join(closeErr, err)
		} else if finalized > 0 {
			log.Printf("databaserunner: finalized %d queued jobs on shutdown", finalized)
		}
	}

	if d.conn != nil {
		if err := d.conn.Close(); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}

	return closeErr
}

func (d *dbrunner) produceSeedJobs(ctx context.Context) error {
	var input io.Reader

	switch d.cfg.InputFile {
	case "stdin":
		input = os.Stdin
	default:
		f, err := os.Open(d.cfg.InputFile)
		if err != nil {
			return err
		}

		defer f.Close()

		input = f
	}

	jobs, err := runner.CreateSeedJobs(
		d.cfg.FastMode,
		d.cfg.LangCode,
		input,
		d.cfg.MaxDepth,
		d.cfg.Email,
		d.cfg.GeoCoordinates,
		d.cfg.Zoom,
		d.cfg.Radius,
		nil,
		nil,
		d.cfg.ExtraReviews,
	)
	if err != nil {
		return err
	}

	for i := range jobs {
		if err := d.provider.Push(ctx, jobs[i]); err != nil {
			return err
		}
	}

	_ = runner.Telemetry().Send(ctx, tlmt.NewEvent("databaserunner.produceSeedJobs", map[string]any{
		"job_count": len(jobs),
	}))

	return nil
}

func openPsqlConn(dsn string) (conn *sql.DB, err error) {
	if !strings.Contains(dsn, "default_query_exec_mode") {
		sep := " "
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			sep = "?"
			if strings.Contains(dsn, "?") {
				sep = "&"
			}
		}
		dsn += sep + "default_query_exec_mode=simple_protocol"
	}

	conn, err = sql.Open("pgx", dsn)
	if err != nil {
		return
	}

	err = conn.Ping()
	if err != nil {
		return
	}

	conn.SetMaxOpenConns(10)

	return
}

func finalizeQueuedJobs(conn *sql.DB, reason string) (int64, error) {
	const q = `
		UPDATE gmaps_jobs
		SET status = 'failed'
		WHERE status = 'queued'
	`

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := conn.ExecContext(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("finalize queued jobs (%s): %w", reason, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected (%s): %w", reason, err)
	}

	return affected, nil
}

