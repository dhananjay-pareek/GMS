const { chromium } = require('playwright');

(async () => {
    const browser = await chromium.launch();
    const page = await browser.newPage();
    await page.goto('https://www.google.com/maps/search/dentist+in+new+york?hl=en', { waitUntil: 'networkidle' });

    // Wait a bit for feed to load
    await page.waitForTimeout(3000);

    // Extract the feed HTML
    const html = await page.evaluate(() => {
        const feed = document.querySelector('div[role="feed"]');
        if (!feed) return "No feed found";
        return feed.innerHTML;
    });

    const fs = require('fs');
    fs.writeFileSync('feed.html', html);
    console.log("Dumped feed to feed.html");
    await browser.close();
})();
