# Deploying Google Maps Scraper to Render

This guide walks you through deploying the Google Maps Scraper to [Render](https://render.com) - a modern cloud platform that makes deployment simple.

## 🚀 Quick Start

### Prerequisites
- A [Render account](https://dashboard.render.com/register) (free tier available)
- Your code pushed to a Git repository (GitHub, GitLab, or Bitbucket)

### Option 1: One-Click Deploy (Recommended)

1. **Fork or push this repository to your GitHub account**

2. **Click the Deploy button** (or manually create from dashboard)
   - Go to [Render Dashboard](https://dashboard.render.com)
   - Click "New +" → "Blueprint"
   - Connect your repository
   - Render will automatically detect `render.yaml` and configure everything

3. **Wait for deployment** (~5-10 minutes for first build)

4. **Access your app** at the provided URL (e.g., `https://your-app-name.onrender.com`)

### Option 2: Manual Setup

#### Step 1: Create Web Service

1. Go to [Render Dashboard](https://dashboard.render.com)
2. Click **"New +"** → **"Web Service"**
3. Connect your Git repository
4. Configure:
   - **Name:** `google-maps-scraper`
   - **Runtime:** `Docker`
   - **Branch:** `main`
   - **Dockerfile Path:** `./Dockerfile`

#### Step 2: Configure Environment Variables

Add these in the "Environment" section:

| Variable | Value | Description |
|----------|-------|-------------|
| `PORT` | (auto-set by Render) | Port the app listens on |
| `DATA_FOLDER` | `/data` | Where to store SQLite DB and results |
| `CONCURRENCY` | `2` | Number of concurrent scrapers (adjust based on plan) |
| `DISABLE_TELEMETRY` | `1` | Disable analytics |

**Optional variables:**
- `DATABASE_URL` - PostgreSQL connection string (if using database mode)
- `PROXIES` - Comma-separated proxy list

#### Step 3: Add Persistent Disk

1. In your service settings, go to **"Disks"**
2. Click **"Add Disk"**
3. Configure:
   - **Name:** `scraper-data`
   - **Mount Path:** `/data`
   - **Size:** `10 GB` (minimum)

⚠️ **Important:** Without a persistent disk, all data will be lost when the service restarts!

#### Step 4: Configure Health Checks

- **Health Check Path:** `/health`
- Render will automatically ping this endpoint to ensure your service is running

#### Step 5: Deploy

Click **"Create Web Service"** and wait for the build to complete (~5-10 minutes).

## 📊 Service Plans

| Plan | RAM | CPU | Price | Recommended For |
|------|-----|-----|-------|-----------------|
| Free | 512 MB | 0.1 CPU | $0/mo | Testing only (spins down after inactivity) |
| Starter | 512 MB | 0.5 CPU | $7/mo | Light usage (1-2 concurrent jobs) |
| Standard | 2 GB | 1 CPU | $25/mo | Moderate usage (4-6 concurrent jobs) |
| Pro | 4 GB | 2 CPU | $85/mo | Heavy usage (10+ concurrent jobs) |

💡 **Tip:** Start with Starter plan and scale up based on your needs.

## 🔧 Configuration

### Environment Variables Reference

#### Core Settings
```bash
PORT                 # Auto-set by Render (e.g., 10000)
DATA_FOLDER=/data    # Persistent disk mount path
CONCURRENCY=2        # Concurrent scraper workers
```

#### Optional Settings
```bash
DISABLE_TELEMETRY=1              # Disable analytics
DATABASE_URL=postgres://...      # PostgreSQL connection (for database mode)
PROXIES=socks5://proxy1,http://proxy2  # Proxy rotation
```

### Using PostgreSQL (Optional)

If you need production-grade persistence beyond SQLite:

1. **Create PostgreSQL database** in Render:
   - Click "New +" → "PostgreSQL"
   - Name: `google-maps-db`
   - Plan: Starter or higher

2. **Link to your web service:**
   - In `render.yaml`, uncomment the `DATABASE_URL` environment variable
   - It will automatically use the database connection string

3. **Update startup command** to use database mode:
   ```bash
   ./google-maps-scraper -dsn $DATABASE_URL
   ```

## 🌐 Accessing Your Deployment

Once deployed, you'll get a URL like: `https://google-maps-scraper-abc123.onrender.com`

### Web Interface
- **Homepage:** `https://your-app.onrender.com/`
- **API Docs:** `https://your-app.onrender.com/api/docs`
- **Health Check:** `https://your-app.onrender.com/health`

### API Endpoints

#### Create a scraping job
```bash
curl -X POST https://your-app.onrender.com/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "Name": "Test Job",
    "Keywords": ["restaurant in paris", "cafe in london"],
    "Lang": "en",
    "Depth": 10,
    "MaxTime": 300,
    "FastMode": false,
    "Email": true
  }'
```

#### Check job status
```bash
curl https://your-app.onrender.com/api/v1/jobs/{job-id}
```

#### Download results
```bash
curl https://your-app.onrender.com/api/v1/jobs/{job-id}/download -o results.csv
```

## ⚙️ Performance Tuning

### Adjusting Concurrency

Based on your Render plan:

| Plan | RAM | Recommended CONCURRENCY |
|------|-----|------------------------|
| Free/Starter (512 MB) | 1-2 |
| Standard (2 GB) | 4-6 |
| Pro (4 GB) | 8-12 |

Set via environment variable:
```bash
CONCURRENCY=4
```

### Disk Space

Results and SQLite database use disk space. Monitor usage:
- Starter: 10 GB disk
- Scale up if needed: Settings → Disks → Resize

## 🐛 Troubleshooting

### Service won't start
**Check logs:** Dashboard → Your Service → Logs

Common issues:
1. **Port binding error** - Ensure app reads `PORT` env var (already configured)
2. **Out of memory** - Upgrade to larger plan or reduce `CONCURRENCY`
3. **Disk full** - Clean old results or increase disk size

### Slow scraping
1. **Increase concurrency** - Set `CONCURRENCY=4` (or higher based on plan)
2. **Use proxies** - Add `PROXIES` env var to avoid rate limiting
3. **Upgrade plan** - More CPU/RAM = faster scraping

### Free tier limitations
- **Auto-spins down** after 15 minutes of inactivity
- **First request slow** (~30 seconds to spin up)
- **Solution:** Upgrade to Starter plan ($7/mo) for always-on service

### Data loss after restart
- **Cause:** No persistent disk configured
- **Solution:** Add disk in Settings → Disks

## 🔒 Security Best Practices

1. **Don't commit secrets** - Use Render's environment variables
2. **Enable HTTPS** - Enabled by default on Render
3. **Use strong proxies** - If scraping at scale
4. **Monitor usage** - Check Render dashboard for resource usage
5. **Backup data** - Download results regularly

## 📈 Scaling

### Horizontal Scaling (Multiple Instances)

In `render.yaml`, increase:
```yaml
numInstances: 3  # Run 3 copies of your service
```

**Note:** With multiple instances, use PostgreSQL instead of SQLite for shared state.

### Vertical Scaling (More Resources)

Upgrade plan:
- Dashboard → Your Service → Settings → Plan
- Choose larger plan for more RAM/CPU

## 💰 Cost Estimation

### Example Scenarios

**Light Usage** (Testing, personal use)
- Starter Web Service: $7/mo
- 10 GB Disk: Included
- **Total: ~$7/mo**

**Moderate Usage** (Small business)
- Standard Web Service: $25/mo
- PostgreSQL Starter: $7/mo
- 20 GB Disk: Included
- **Total: ~$32/mo**

**Heavy Usage** (Production, multiple users)
- Pro Web Service: $85/mo
- PostgreSQL Standard: $20/mo
- 50 GB Disk: +$10/mo
- **Total: ~$115/mo**

## 📚 Additional Resources

- [Render Documentation](https://render.com/docs)
- [Docker on Render](https://render.com/docs/docker)
- [Environment Variables](https://render.com/docs/environment-variables)
- [Persistent Disks](https://render.com/docs/disks)
- [PostgreSQL on Render](https://render.com/docs/databases)

## 🆘 Support

- **Render Support:** [https://render.com/support](https://render.com/support)
- **GitHub Issues:** [Create an issue](https://github.com/dhananjay-pareek/google-maps-scraper/issues)
- **Community:** [Render Community](https://community.render.com)

---

**Happy Scraping! 🚀**
