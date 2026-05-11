# GitHub Copilot CLI — Developer Guide

This project is set up to work well with `gh copilot` for day-to-day development tasks.

## Installation

```bash
# Install GitHub CLI
winget install --id GitHub.cli -e

# Install the Copilot extension
gh extension install github/gh-copilot

# Authenticate
gh auth login
```

## Recommended shell aliases

Add these to your PowerShell profile (`$PROFILE`):

```powershell
Set-Alias -Name gcs -Value { gh copilot suggest $args }
Set-Alias -Name gce -Value { gh copilot explain $args }
```

Or for bash/zsh:

```bash
alias gcs='gh copilot suggest'
alias gce='gh copilot explain'
```

## Usage patterns

### Suggest a command

```bash
gh copilot suggest "run the collector tests with the race detector and verbose output"
gh copilot suggest "apply only the first migration to the local database"
gh copilot suggest "build the collector Docker image and tag it as latest"
gh copilot suggest "stream logs from the predictor container only"
gh copilot suggest "force-recreate the predictor container without rebuilding the image"
```

### Explain an unfamiliar command

```bash
gh copilot explain "docker compose --profile tracing up -d --no-deps collector"
gh copilot explain "go test -race -count=1 -run TestIngest ./internal/service/..."
gh copilot explain "migrate -path db/migrations -database $DB_URL force 1"
```

### Makefile targets that wrap Copilot CLI

```bash
make copilot-suggest   # opens interactive suggest prompt
make copilot-explain   # opens interactive explain prompt
```

## VS Code integration

Open the **Terminal** panel and run `gh copilot suggest` inline.  
The VS Code task **"Copilot CLI: Suggest command"** (in `.vscode/tasks.json`) launches it in a
dedicated terminal panel — press `Ctrl+Shift+P → Run Task` to invoke it.

## Tips for this project

- Always name the service you're targeting: *"for the Go collector service"*, *"in the predictor container"*.
- Reference Makefile targets: *"equivalent to `make go-test` but only for the handler package"*.
- For Docker Compose questions, mention the profile: *"using the tracing profile"*.
