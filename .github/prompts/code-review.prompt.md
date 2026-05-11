---
mode: agent
description: Perform a thorough code review of the current file or selection
---

Perform a code review on the selected code or the file at **`${input:targetFile}`**.

## Review checklist

### Correctness
- [ ] Logic errors or off-by-one mistakes
- [ ] Unhandled error cases or swallowed errors
- [ ] Race conditions or data races (especially in Go)
- [ ] Nil/null pointer dereferences

### Security (OWASP Top 10)
- [ ] SQL injection — are all queries parameterized?
- [ ] Secrets or credentials hardcoded or logged
- [ ] Input validation at service boundaries
- [ ] Dependency vulnerabilities (note any suspicious imports)

### Observability
- [ ] Missing OTel spans on significant operations
- [ ] Errors not recorded on spans
- [ ] Missing structured log fields (trace_id, span_id)

### Performance
- [ ] N+1 query patterns
- [ ] Missing database indexes for query patterns
- [ ] Unbounded memory allocations

### Maintainability
- [ ] Functions > 50 lines that should be split
- [ ] Duplicate logic that should be extracted
- [ ] Missing or misleading variable/function names

### Project conventions
- [ ] Follows the conventions in `.github/copilot-instructions.md`
- [ ] Context propagation correct (Go)
- [ ] Type annotations present (Python)

## Output format

For each issue found, provide:
1. **Severity**: Critical / High / Medium / Low
2. **Location**: file + line number
3. **Issue**: description
4. **Suggestion**: corrected code snippet
