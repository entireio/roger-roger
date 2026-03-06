# roger-roger

A minimal deterministic test agent for the Entire CLI external agent protocol. No API calls — just regex-based prompt parsing.

## What it does

- **"create a file called X"** → creates the file
- **Anything else** → responds "roger roger"

## Build

```
mise run build
```

Produces two binaries:

- `roger-roger` — interactive agent REPL
- `entire-agent-roger-roger` — Entire CLI external agent protocol handler

## Usage

```
./roger-roger
> create a file called hello.txt
Created hello.txt.
> what's up
roger roger
> exit
```

## Entire CLI integration

Place `entire-agent-roger-roger` on `$PATH`. It will be auto-discovered when `external_agents: true` is set in `.entire/settings.json`.
