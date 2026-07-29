---
name: browser-validation
description: Browser validation, visual QA, console debugging, network inspection, or responsive checks for frontend changes. Use when a task needs manual browser verification or diagnosing a browser-only issue.
---

# Browser Validation

Start the relevant local service using the project's documented commands and confirm its URL before connecting a browser.

Use `playwright` MCP for deterministic UI validation:

- Navigate, interact through accessible page state, and capture screenshots.
- Inspect accessibility snapshots before relying on coordinate-based actions.
- Validate the changed path at desktop and narrow mobile viewports.
- Check keyboard access, visible focus, labels, and primary error or empty states when relevant.

Use `chrome-devtools` MCP for live Chromium diagnostics:

- Inspect console errors and warnings, failed or slow network requests, and runtime state.
- Use performance tooling only for a reported or observable performance issue.
- Avoid exposing authenticated data, tokens, or user-entered secrets in tool output.

Report the URL and viewport(s) checked, meaningful browser evidence, and any limitations. Do not claim browser verification when the local application was not running or the browser could not reach it.
