//go:build !rod

package main

import (
	"github.com/dhananjay-pareek/google-maps-scraper/internal/scrapemate"
	jsfetcher "github.com/dhananjay-pareek/google-maps-scraper/internal/scrapemate/adapters/fetchers/jshttp"
)

func newJSFetcher(concurrency int, rotator scrapemate.ProxyRotator, _ bool) (scrapemate.HTTPFetcher, error) {
	opts := jsfetcher.JSFetcherOptions{
		Headless:      false,
		DisableImages: false,
		Rotator:       rotator,
		PoolSize:      concurrency,
	}
	return jsfetcher.New(opts)
}
