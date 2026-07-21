# OpenCode

This repository includes a project-scoped OpenCode setup:

- `opencode.json` loads the repository's `AGENTS.md` guidance and disables session sharing.
- `.opencode/agent/` provides `architect`, `coder`, `debugger`, `devops`, `frontend`,
  `reviewer`, and `coordinator` subagents.
- `.opencode/command/` ports the repository's Copilot workflows as OpenCode slash commands.
- `.opencode/skills/` provides context-specific guidance for Go, Airflow Python, frontend,
  and SQL migration work.

The existing `.github/` Copilot files remain the canonical shared source for detailed
agent instructions and workflow prompts. The OpenCode configuration refers to them, so
updates apply to both tools.

## Install

Use WSL for this repository. From a WSL terminal, install OpenCode with one of the
official methods:

```bash
curl -fsSL https://opencode.ai/install | bash
```

or, with Node.js:

```bash
npm install -g opencode-ai
```

Confirm it is available:

```bash
opencode --version
```

## Authenticate

Start OpenCode from the repository root:

```bash
opencode
```

In the OpenCode TUI, run `/connect`, then select and authenticate with an available
provider. Provider credentials are user-specific and must not be committed to this
repository. Select your preferred model after connecting; this project intentionally
does not force a provider or model.

## Use

OpenCode automatically loads `AGENTS.md`, `opencode.json`, agents, commands, and skills
when started at the repository root. Start a normal task directly, or delegate a focused
task with an agent mention such as `@reviewer review services/go/collector`.

Available project commands:

```text
/add-tracing <target>
/code-review <target>
/design-api-spec <service name, purpose, language>
/implement-from-spec <spec path>
/new-airflow-dag <DAG ID, description, schedule>
/new-db-migration <description, slug>
/new-go-service <service name>
/new-python-service <Airflow processing request>
/write-tests <target>
```

`/new-python-service` is intentionally retained only as a migration-friendly command
name. It creates or changes Airflow processing code instead of the obsolete standalone
FastAPI service described by the old Copilot prompt.

## Updating Configuration

After changing `opencode.json`, anything under `.opencode/`, or `AGENTS.md`, quit and
restart OpenCode. Configuration is loaded only when a session starts.

For personal preferences, use the global config at `~/.config/opencode/opencode.json`.
Do not add API keys, provider credentials, or machine-specific paths to the project
configuration.

If a future project configuration prevents OpenCode from starting, launch it once with:

```bash
OPENCODE_DISABLE_PROJECT_CONFIG=1 opencode
```

Fix the configuration, then restart normally.
