---
description: Create Airflow plugin or DAG processing code from existing contracts.
agent: coder
---

The legacy prompt at `.github/prompts/new-python-service.prompt.md` targets an obsolete standalone FastAPI service. Do not follow it. Current Python work belongs in `airflow/dags/` and `airflow/plugins/pociag_processing/`. Read the `python-airflow` skill, inspect the relevant specs, and implement the request: $ARGUMENTS
