# shared

Small Go module imported by `data-service` and `gateway` through a
`replace github.com/pociag-do-predykcji/services/go/shared => ../shared` directive in their
`go.mod`. Edits here take effect immediately in both consumers — no versioning/publish.

## Packages

- `dsmodel/model.go` — data structures shared across the data-service/gateway boundary.
- `trainutil/util.go` — train-domain helpers.

## Rules

- Keep it **dependency-free** (`go.mod` has no `require` block). Don't pull in chi, pgx, otel, etc.
- `collector` does **not** use this module — don't add collector-only code here.
- No Make targets: test with `cd services/go/shared && go test -race ./...`. Not linted in CI.
- Changing an exported type here can break both consumers — build them after
  (`make data-service-test gateway-test`).
