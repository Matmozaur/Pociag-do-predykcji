---
mode: agent
description: Write comprehensive tests for a given file or function
---

Write tests for: **`${input:targetFile}`** (or the currently selected code).

## Testing strategy

1. **Unit tests** — test each function/method in isolation with mocked dependencies.
2. **Integration tests** — test against real PostgreSQL using testcontainers (mark with build tag `integration` in Go, or `@pytest.mark.integration` in Python).
3. **Edge cases** — empty inputs, boundary values, error paths, concurrent access where relevant.

## Go test conventions

```go
// File: internal/service/service_test.go
package service_test

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestServiceName_Scenario_ExpectedBehavior(t *testing.T) {
    t.Parallel()
    // Arrange
    // Act
    // Assert
}
```

- Use `testify/assert` and `testify/require`.
- Mock interfaces with `github.com/stretchr/testify/mock` or `gomock`.
- Integration tests: spin up Postgres with `testcontainers-go`, run migrations, seed data, then test.
- Test file sits next to the file under test.

## Python test conventions

```python
# tests/test_<module>.py
import pytest

class TestClassName:
    def test_method_scenario_expected(self, ...):
        # Arrange
        # Act
        # Assert
```

- Use `pytest` with `pytest-asyncio` for async tests.
- Use `unittest.mock.AsyncMock` for mocking async functions.
- Integration tests: use `testcontainers` Postgres fixture in `conftest.py`.
- Mark slow tests with `@pytest.mark.slow`.

## Coverage target

Ensure all generated tests together reach ≥ 80% line coverage on the file under test.
List any code paths intentionally left untested and explain why.
