#!/usr/bin/env node

const puppeteer = require('puppeteer');
const path = require('path');
const fs = require('fs');

const FRONTEND_URL = process.env.FRONTEND_URL || 'http://localhost:8080';
const GREEN = '\x1b[0;32m';
const RED = '\x1b[0;31m';
const YELLOW = '\x1b[1;33m';
const NC = '\x1b[0m';

// Test result tracking
let totalTests = 0;
let passedTests = 0;
let failedTests = 0;

async function runTest(name, testFn) {
    totalTests++;
    
    // Log test start with timestamp
    const startTime = new Date();
    const timestamp = startTime.toLocaleTimeString();
    console.log(`\n[${timestamp}] Starting test: ${name}`);
    process.stdout.write(`Test: ${name}... `);
    
    let timeoutId;
    // Create a promise that rejects after 1 minute
    const timeoutPromise = new Promise((_, reject) => {
        timeoutId = setTimeout(() => {
            reject(new Error(`Test timeout after 60 seconds: ${name}`));
        }, 60000); // 1 minute = 60000 milliseconds
    });
    
    try {
        // Race between the test and the timeout
        const result = await Promise.race([
            testFn().then(result => {
                // Clear timeout if test completes successfully
                clearTimeout(timeoutId);
                return result;
            }),
            timeoutPromise
        ]);
        passedTests++;
        const duration = ((new Date() - startTime) / 1000).toFixed(2);
        console.log(`${GREEN}✓ PASSED${NC} (${duration}s)`);
        return true;
    } catch (error) {
        // Clear timeout if test fails
        clearTimeout(timeoutId);
        failedTests++;
        const duration = ((new Date() - startTime) / 1000).toFixed(2);
        console.log(`${RED}✗ FAILED${NC} (${duration}s)`);
        console.log(`  Error: ${error.message}`);
        return false;
    }
}

async function loadTestModules() {
    const testsDir = __dirname;
    const testFiles = fs.readdirSync(testsDir)
        .filter(file => file.endsWith('.test.js'))
        .sort();

    const tests = [];
    for (const file of testFiles) {
        const testModule = require(path.join(testsDir, file));
        tests.push({ file, module: testModule });
    }

    return tests;
}

async function main() {
    console.log('='.repeat(60));
    console.log('todo-frontend Integration Tests');
    console.log('='.repeat(60));

    let browser;

    try {
        console.log('\nLaunching headless browser...');
        // Prefer the configured executable path when it exists, otherwise fall back to Puppeteer's default.
        const arch = process.arch;
        const preferred = [];
        const fallback = [];
        const pushPreferred = (p) => p && fs.existsSync(p) && preferred.push(p);
        const pushFallback = (p) => p && fs.existsSync(p) && fallback.push(p);

        if (process.env.PUPPETEER_EXECUTABLE_PATH) {
            pushPreferred(process.env.PUPPETEER_EXECUTABLE_PATH);
        }

        // Common cached chromium locations (order matters: prefer native arch to avoid Rosetta issues)
        const chromeCacheRoot = '/tmp/.cache/puppeteer/chrome';
        if (fs.existsSync(chromeCacheRoot)) {
            const versions = fs.readdirSync(chromeCacheRoot);
            versions.forEach((ver) => {
                const base = path.join(chromeCacheRoot, ver);
                const arm = path.join(base, 'chrome-linux-arm64', 'chrome');
                const x64 = path.join(base, 'chrome-linux64', 'chrome');
                if (arch === 'arm64') {
                    pushPreferred(arm);
                    pushFallback(x64); // only use x64 if nothing else works
                } else {
                    pushPreferred(x64);
                    pushFallback(arm);
                }
            });
        }

        // Fallback to puppeteer's detected path (may download if bundled)
        const detected = puppeteer.executablePath();
        pushFallback(detected);

        const systemCandidates = [
            '/usr/bin/chromium-browser',
            '/usr/bin/chromium',
            '/usr/bin/google-chrome',
            '/usr/bin/chrome',
        ];
        systemCandidates.forEach(pushPreferred);

        const candidatePaths = [...preferred, ...fallback];
        let executablePath = candidatePaths.find(Boolean);
        if (!executablePath) {
            throw new Error('No Chromium/Chrome executable found for Puppeteer');
        }

        browser = await puppeteer.launch({
            headless: true,
            args: [
                '--no-sandbox',
                '--disable-setuid-sandbox',
                '--disable-dev-shm-usage'
            ],
            executablePath,
            protocolTimeout: 120000  // generous protocol timeout to avoid flakiness
        });
        console.log(`${GREEN}✓ Browser launched${NC}`);

        // Load all test modules
        const testModules = await loadTestModules();

        if (testModules.length === 0) {
            console.log(`${YELLOW}⚠ No test files found${NC}`);
            return;
        }

        console.log(`\nFound ${testModules.length} test file(s)`);

        // Run tests from each module
        for (const { file, module } of testModules) {
            console.log(`\n${'='.repeat(60)}`);
            console.log(`Running tests from: ${file}`);
            console.log('='.repeat(60));

            if (typeof module.runTests === 'function') {
                await module.runTests(browser, FRONTEND_URL, runTest);
            } else {
                console.log(`${YELLOW}⚠ No runTests function exported from ${file}${NC}`);
            }
        }

        // Print summary
        console.log('\n' + '='.repeat(60));
        console.log('Frontend Test Summary:');
        console.log(`  Total:  ${totalTests}`);
        console.log(`  ${GREEN}Passed: ${passedTests}${NC}`);
        console.log(`  ${RED}Failed: ${failedTests}${NC}`);
        console.log('='.repeat(60));

        if (failedTests > 0) {
            process.exit(1);
        } else {
            console.log(`\n${GREEN}✓ ALL TESTS PASSED${NC}\n`);
            process.exit(0);
        }

    } catch (error) {
        console.error(`${RED}✗ Fatal error: ${error.message}${NC}`);
        console.error(error.stack);
        process.exit(1);
    } finally {
        if (browser) {
            await browser.close();
        }
    }
}

main();
