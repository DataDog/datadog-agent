/**
 * Basic UI Tests
 * Tests the fundamental UI elements and page structure
 * 
 * PUPPETEER MIGRATION NOTE:
 * -------------------------
 * page.waitForTimeout() was removed in Puppeteer v22.0.0+
 * 
 * DO NOT USE:
 *   await page.waitForTimeout(1000);  // DEPRECATED/REMOVED
 * 
 * USE INSTEAD:
 *   // Wait for elements to appear
 *   await page.waitForSelector('.my-element');
 *   await page.waitForSelector('#myId', { visible: true });
 *   
 *   // Wait for element to disappear
 *   await page.waitForSelector('.loading', { hidden: true });
 *   
 *   // Wait for specific conditions
 *   await page.waitForFunction(() => document.querySelector('#el').innerText.includes('Ready'));
 *   
 *   // Wait for navigation/network
 *   await page.waitForNavigation({ waitUntil: 'networkidle0' });
 *   await page.waitForResponse(res => res.url().includes('/api/') && res.status() === 200);
 *   
 *   // If you absolutely need a delay (not recommended), use:
 *   await new Promise(resolve => setTimeout(resolve, 1000));
 */

// !!!! LLM NOTE: PLEASE CUSTOMIZE THE BASIC UI TESTS FOR YOUR APPLICATION, AND REMOVE THIS NOTE.

async function runTests(browser, frontendUrl, runTest) {
    const page = await browser.newPage();
    page.setDefaultNavigationTimeout(60000);
    page.setDefaultTimeout(30000);
    
    // Track console errors
    const consoleErrors = [];

    // Enable console logging from browser
    page.on('console', (msg) => {
        if (msg.type() === 'error') {
            const errorText = msg.text();
            consoleErrors.push(errorText);
            console.log(`    Browser console error: ${errorText}`);
        }
    });

    // Test 1: Homepage loads successfully
    await runTest('Homepage loads successfully', async () => {
        const response = await page.goto(frontendUrl, {
            waitUntil: 'networkidle2',
            timeout: 15000
        });

        if (!response.ok()) {
            throw new Error(`HTTP ${response.status()}`);
        }
    });

    // Test 2: Page should contain the word 'GenSim'
    await runTest('Page should contain the word "GenSim"', async () => {
        const content = await page.$eval('body', el => el.textContent);
        if (!content.includes('GenSim')) {
            throw new Error(`Content should include "GenSim": ${content}`);
        }
    });

    // Test 3: No console errors
    await runTest('No console errors', async () => {
        if (consoleErrors.length > 0) {
            throw new Error(`Found ${consoleErrors.length} console error(s):\n${consoleErrors.map((e, i) => `  ${i + 1}. ${e}`).join('\n')}`);
        }
    });

    // ... add more tests here ...

    await page.close();
}

module.exports = { runTests };
