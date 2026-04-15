package filerunner

import (
	"context"

	"github.com/gosom/scrapemate"
)

type multiWriter struct {
	writers []scrapemate.ResultWriter
}

func newMultiWriter(w ...scrapemate.ResultWriter) scrapemate.ResultWriter {
	return &multiWriter{writers: w}
}

func (m *multiWriter) Run(ctx context.Context, in <-chan scrapemate.Result) error {
	chans := make([]chan scrapemate.Result, len(m.writers))
	errs := make(chan error, len(m.writers))

	for i, w := range m.writers {
		chans[i] = make(chan scrapemate.Result, 100)
		go func(writer scrapemate.ResultWriter, ch <-chan scrapemate.Result) {
			errs <- writer.Run(ctx, ch)
		}(w, chans[i])
	}

	for result := range in {
		for _, ch := range chans {
			select {
			case <-ctx.Done():
				break
			case ch <- result:
			}
		}
	}

	for _, ch := range chans {
		close(ch)
	}

	var firstErr error
	for i := 0; i < len(m.writers); i++ {
		if err := <-errs; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
