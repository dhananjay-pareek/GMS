# GMAP Scraper

> ✅ **Repository synced on March 30, 2026** - All files updated from source repository

A powerful and open-source Google Maps scraper for extracting business data at scale. Available as CLI, Web UI, REST API.

## Why Use This Scraper?

| | |
|---|---|
| **Multiple Interfaces** | CLI, Web UI, REST API - use what fits your workflow |
| **High Performance** | ~120 places/minute with optimized concurrency |
| **33+ Data Points** | Business details, reviews, emails, coordinates, and more |
| **Production Ready** | Scale from a single machine to Kubernetes clusters |
| **Flexible Output** | CSV, JSON, PostgreSQL, S3, or custom plugins |
| **Proxy Support** | Built-in SOCKS5/HTTP/HTTPS proxy rotation |

## Quick Start

### Web UI

Start the web interface with a single command:

```bash
mkdir -p gmapsdata && docker run -v $PWD/gmapsdata:/gmapsdata -p 8080:8080 gosom/google-maps-scraper -data-folder /gmapsdata
```

Then open http://localhost:8080 in your browser.

> **Note:** Results take at least 3 minutes to appear (minimum configured runtime).
> 
> **macOS Users:** Docker command may not work. See [MacOS Instructions](MacOS%20instructions.md).

### Command Line

```bash
touch results.csv && docker run \
  -v $PWD/example-queries.txt:/example-queries \
  -v $PWD/results.csv:/results.csv \
  gosom/google-maps-scraper \
  -depth 1 \
  -input /example-queries \
  -results /results.csv \
  -exit-on-inactivity 3m
```

**Want emails?** Add the `-email` flag.

**Want all reviews (up to ~300)?** Add `--extra-reviews` and use `-json` output.

## Installation

### Using Docker (Recommended)

Two Docker image variants are available:

| Image | Tag | Browser Engine | Best For |
|-------|-----|----------------|----------|
| Playwright (default) | `latest`, `vX.X.X` | Playwright | Most users, better stability |
| Rod | `latest-rod`, `vX.X.X-rod` | Rod/Chromium | Lightweight, faster startup |

```bash
# Playwright version (default)
docker pull gosom/google-maps-scraper

# Rod version (alternative)
docker pull gosom/google-maps-scraper:latest-rod
```

### Build from Source

Requirements: Go 1.25.6+

```bash
git clone https://github.com/gosom/GMAP-Scrapper.git
cd GMAP-Scrapper
go mod download

# Playwright version (default)
go build
./google-maps-scraper -input example-queries.txt -results results.csv -exit-on-inactivity 3m

# Rod version (alternative)
go build -tags rod
./google-maps-scraper -input example-queries.txt -results results.csv -exit-on-inactivity 3m
```

> First run downloads required browser libraries (Playwright or Chromium depending on version).

## Leads Manager from the scraper binary

You can now launch Leads Manager directly from the built scraper binary:

```bash
./google-maps-scraper -leads-manager
```

Optional flags:
- `-leads-manager-addr :9090` to change the bind address (default `:9090`)
- `-leads-db-path webdata\leadsmanager.db` to change local SQLite path (default `webdata\leadsmanager.db`)

Scraped results are written only to the local Leads DB automatically in both file mode and web mode.

## Run both Web UI and Leads Manager together (default)

When you start without explicit mode flags, the binary starts both:
- Scraper Web UI on `-addr` (default `:8080`)
- Leads Manager on `-leads-manager-addr` (default `:9090`)
- Both open as app windows (Edge/Chrome `--app`) on desktop.

```bash
./google-maps-scraper
```

You can also force this behavior explicitly:

```bash
./google-maps-scraper -both
```

Leads Manager includes an **Open Scraper** button in the top bar.

Useful option:
- `-scraper-url` (or `SCRAPER_URL`) controls where the button points (default `http://localhost:8080`)
- `-leads-manager-url` (or `LEADS_MANAGER_URL`) controls Scraper's "Open Leads Manager" button (default `http://localhost:9090`)

## Shared LLM/Ollama config (single app)

You can configure AI once and reuse it in both Scraper Web UI and Leads Manager:

- `LLM_PROVIDER` (default: `ollama`)
- `LLM_API_KEY` (for cloud providers; optional for Ollama)
- `LLM_MODEL` (optional model override)
- `OLLAMA_URL` (default: `http://localhost:11434`)

Equivalent flags:
- `-llm-provider`
- `-llm-api-key`
- `-llm-model`
- `-ollama-url`

Example for laptop Ollama:

```bash
LLM_PROVIDER=ollama \
OLLAMA_URL=http://localhost:11434 \
LLM_MODEL=qwen3-coder:480b-cloud \
./google-maps-scraper
```

## PageSpeed enrichment

The Speed action in Leads Manager uses Google PageSpeed Insights.
For higher reliability, set:

```bash
PAGESPEED_API_KEY=your_google_pagespeed_key
```

If you still hit occasional timeouts, retry once: the app now automatically retries transient PageSpeed timeout/5xx/429 responses.
