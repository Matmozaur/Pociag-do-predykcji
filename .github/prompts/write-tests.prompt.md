---
mode: agent
description: Write comprehensive tests for a given file or function
---

Write tests for: **`${input:targetFile}`** (or the currently selected code).

## Testing strategy

1. **Unit tests** — test each function/method in isolation, with the repo's fakes instead of real dependencies.
2. **SQL tests** — repository code runs against a real disposable PostgreSQL named by `POCIAG_TEST_DATABASE_URL`; the test skips when it is unset and recreates that database's `public` schema, then applies `db/migrations/001_init.up.sql`. No testcontainers, no build tags.
3. **Edge cases** — empty inputs, boundary values, error paths, concurrent access where relevant.

## Go test conventions (`services/go/api`)

```go
// File: internal/position/position_test.go
package position

import "testing"

func TestEstimate_NoStops(t *testing.T) {
    t.Parallel()
    cases := []struct {
        name string
        // inputs, want
    }{
        // ...
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            // Arrange / Act / Assert with t.Fatalf / t.Errorf
        })
    }
}
```

- Standard library `testing` only — no testify or gomock (not in `go.mod`).
- Service tests use the fake repository in `internal/service/servicetest` and the fixed clock from `internal/service/export_test.go`.
- Handler tests drive `Handler.Routes()` with `net/http/httptest`.
- SQL tests live in `internal/repository/repository_test.go` (see its setup helper).
- Test names: `Test<Thing>` or `Test<Thing>_<Case>`. Test file sits next to the file under test.
- Run: `make api-test` (`go test -race ./...`).

## Python test conventions (`airflow/`)

```python
# tests/test_<module>.py
from __future__ import annotations


def test_function_scenario() -> None:
    # Arrange
    # Act
    # Assert
    ...
```

- `pytest` (+ `pytest-mock`); plain test functions, annotated like the existing tests (ruff checks `tests/`; mypy runs on `plugins/` and `dags/`).
- `transform.py` tests use trimmed real PLK payloads from `tests/fixtures/`.
- `pipelines.py` tests inject fakes (`FakePlk`, `RecordingRepository` in `tests/test_pipelines.py`).
- SQL tests go in `tests/test_repository_integration.py` (module-level `skipif` on `POCIAG_TEST_DATABASE_URL`).
- Run: `make airflow-test`.

## Coverage target

Ensure all generated tests together reach ≥ 80% line coverage on the file under test.
List any code paths intentionally left untested and explain why.
