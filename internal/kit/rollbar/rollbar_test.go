package rollbar_test

import (
	"testing"

	"github.com/dhananjay-pareek/google-maps-scraper/internal/kit/lib"
	"github.com/dhananjay-pareek/google-maps-scraper/internal/kit/rollbar"
	"github.com/stretchr/testify/require"
)

func TestNewRollbarErrorReporter(t *testing.T) {
	t.Run("TestThatNewRollbarErrorReporterReturnsErrorReporter", func(t *testing.T) {
		reporter := rollbar.NewRollbarErrorReporter(
			"fooToken",
			"fooEnv",
			"fooversion",
			"fooServerHost",
			"fooServerRoot",
		)
		require.Implements(t, (*lib.ErrorReporter)(nil), reporter)
	})
}
