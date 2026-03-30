# GMAP Scrapper (gmap-new)

**GMAP Scrapper** is a high-speed, automated tool designed to extract comprehensive business data from Google Maps. Whether you need to build a lead list, conduct market research, or analyze local competitors, this tool transforms raw Google Maps results into structured, actionable data.

Available as a **Command Line Tool (CLI)**, a user-friendly **Web Interface**, and a **REST API**.

## 🚀 Key Features

- **📧 Email Discovery**: Automatically finds business email addresses associated with Google Maps listings.
- **⭐ Full Review Extraction**: Scrapes up to 300 reviews per location, allowing for deep sentiment analysis.
- **📊 33+ Data Points**: Extracts everything from coordinates and social profiles to ratings and business hours.
- **🔌 Flexible Export**: Save your data to CSV, JSON, PostgreSQL, or even S3.
- **🛡️ Production Ready**: Built-in support for proxy rotation and high-concurrency scraping.

## 💡 Common Use Cases

1. **Lead Generation**: Instantly build targeted B2B prospect lists for your sales team.
2. **Market Research**: Analyze ratings, reviews, and categories of competitors in any area.
3. **Local SEO**: Monitor and audit local business listings for optimization opportunities.
4. **Data Enrichment**: Enhance your existing CRM data with fresh details from Google Maps.

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
mkdir -p gmapsdata && docker run -v $PWD/gmapsdata:/gmapsdata -p 8080:8080 ghcr.io/dhananjay-pareek/gmap-scrapper:latest -data-folder /gmapsdata
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
  ghcr.io/dhananjay-pareek/gmap-scrapper:latest \
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
docker pull ghcr.io/dhananjay-pareek/gmap-scrapper:latest

# Rod version (alternative)
docker pull ghcr.io/dhananjay-pareek/gmap-scrapper:latest-rod
```

### Deploy to Render (One-Click Cloud Hosting)

[![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy)

Host your scraper in the cloud with persistent storage and scalable infrastructure.

**Quick Setup:**
1. Click the "Deploy to Render" button above
2. Configure environment variables (PORT, DATA_FOLDER, CONCURRENCY)
3. Wait ~5 minutes for deployment
4. Access your web interface at the provided URL

📖 **[Full Render Deployment Guide →](RENDER.md)**

Features on Render:
- ✅ Always-on service (no cold starts on paid plans)
- ✅ Persistent disk storage for results
- ✅ Auto-scaling and load balancing
- ✅ Free tier available for testing
- ✅ One-click PostgreSQL database

### Build from Source

Requirements: Go 1.25.6+

```bash
git clone https://github.com/dhananjay-pareek/GMAP-Scrapper.git
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