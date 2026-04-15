package webandleadsrunner

import (
	"context"

	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/runner/leadsmanagerrunner"
	"github.com/gosom/google-maps-scraper/runner/webrunner"
	"golang.org/x/sync/errgroup"
)

type webAndLeadsRunner struct {
	webRunner   runner.Runner
	leadsRunner runner.Runner
}

func New(ctx context.Context, cfg *runner.Config) (runner.Runner, error) {
	webCfg := *cfg
	webCfg.RunMode = runner.RunModeWeb
	wr, err := webrunner.New(&webCfg)
	if err != nil {
		return nil, err
	}

	leadsCfg := *cfg
	leadsCfg.RunMode = runner.RunModeLeadsManager
	lr, err := leadsmanagerrunner.New(ctx, &leadsCfg)
	if err != nil {
		_ = wr.Close(ctx)
		return nil, err
	}

	return &webAndLeadsRunner{
		webRunner:   wr,
		leadsRunner: lr,
	}, nil
}

func (w *webAndLeadsRunner) Run(ctx context.Context) error {
	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return w.webRunner.Run(egCtx)
	})
	eg.Go(func() error {
		return w.leadsRunner.Run(egCtx)
	})

	return eg.Wait()
}

func (w *webAndLeadsRunner) Close(ctx context.Context) error {
	if err := w.webRunner.Close(ctx); err != nil {
		return err
	}

	return w.leadsRunner.Close(ctx)
}
