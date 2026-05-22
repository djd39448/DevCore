# DevCore — Canonical Memory Index

Tier 1 memory: the git-versioned source of truth for what DevCore knows.
This file is the map. Read it first; retrieve only what a task needs.

## Layout

| Directory | Holds |
|---|---|
| `architecture/` | DevCore engine & system design notes |
| `decisions/` | Architecture Decision Records (`NNNN-title.md`) |
| `domain/` | The current workload's domain knowledge |
| `conventions/` | The DevCore coding standard (mirror of `/CODING_STANDARDS.md`) |
| `contract/` | The shared contract agents converge on (API + data model) |

## Canonical documents

| Document | Summary |
|---|---|
| `conventions/devcore-coding-standards.md` | DevCore coding standard (`dc-00`–`dc-07`) — the non-negotiable bar |

Architecture notes, ADRs, domain knowledge, and the contract are added as the
build progresses. Episodic memory — past runs and decisions — lives in the
SQLite store (`.devcore/state/episodic.sqlite`) and is queried via the
`devcore-memory` MCP server, not here.
