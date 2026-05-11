---
mode: agent
description: Add OpenTelemetry tracing to an existing service
---

Add OpenTelemetry distributed tracing to the code in the current file or the file at **`${input:targetFile}`**.

## What to instrument

Identify all of the following and add tracing spans:
1. **HTTP handlers** — start a span from the incoming request context; set span name `<resource>.<action>`.
2. **Database calls** — wrap each query in a child span named `db.<table>.<operation>`.
3. **External HTTP calls** — inject W3C TraceContext headers using the OTel propagator.
4. **Business logic functions** — add spans to functions with meaningful latency or error semantics.

## Go implementation

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
)

ctx, span := otel.Tracer("pociag.<service>").Start(ctx, "span.name")
defer span.End()

// On error:
span.RecordError(err)
span.SetStatus(codes.Error, err.Error())

// Add attributes:
span.SetAttributes(attribute.Int64("record.id", id))
```

## Python implementation

```python
from opentelemetry import trace

tracer = trace.get_tracer("pociag.<service>")

with tracer.start_as_current_span("span.name") as span:
    span.set_attribute("record.id", record_id)
    try:
        ...
    except Exception as e:
        span.record_exception(e)
        span.set_status(trace.StatusCode.ERROR, str(e))
        raise
```

## Rules

- Service name attribute: `pociag.<service>` (lowercase).
- Span names: `<noun>.<verb>` pattern, lowercase with dots.
- Always set error status on spans when exceptions propagate.
- Do not create spans for trivial getters/setters.
- Preserve all existing functionality — only add instrumentation.
