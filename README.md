# Veloce

Veloce is a CLI agent that migrates a Laravel monolith (PHP + Blade) into a Go REST backend + a Flutter cross-platform frontend. It orchestrates Gemini 2.5 Flash (95% of calls) and Gemini 2.5 Pro (5% of calls, escalation) through a ReAct loop with per-file checkpoints and a strict token budget kill switch.

See `docs/superpowers/specs/2026-05-29-migration-agent-design.md` for the full design and `docs/superpowers/plans/2026-05-29-veloce-migration-agent.md` for the implementation plan.

## Build

```bash
go build -o veloce ./cmd/veloce
```

## Usage

```bash
export GEMINI_API_KEY=...

./veloce migrate \
  --source  ./my-laravel-project \
  --output  ./output \
  --workers 8 \
  --budget  5000000

./veloce status --output ./output
./veloce retry  --output ./output --file app/Models/User.php
./veloce migrate --source ./my-laravel-project --output ./output --resume
```

### Flags (`migrate`)

| Flag | Default | Description |
|---|---|---|
| `--source` | — | Path to the Laravel project (required) |
| `--output` | `./output` | Output monorepo path |
| `--workers` | 5 | Worker pool size per phase |
| `--budget` | 5 000 000 | Token budget (kill switch) |
| `--api-key` | `$GEMINI_API_KEY` | Gemini API key |
| `--resume` | false | Resume from last checkpoint |
| `--dry-run` | false | Skip API + writes |
| `--run-tests` | false | Run `go test` / `flutter test` after each file |

## State files (under `./output/.veloce/`)

| File | Purpose |
|---|---|
| `migration_state.json` | Per-file checkpoint (status / phase / attempts) |
| `shared_types.json`    | Go/Dart type index for cross-target consistency |
| `token_usage.json`     | Token counts + kill-switch budget |
| `context_cache_id.txt` | Gemini cache id (rules + types) |

## Generated layout

```
output/
├── backend/   # Go (Chi + GORM + JWT)
│   ├── cmd/api/
│   ├── internal/{handler,service,repository,domain}/
│   ├── pkg/{auth,middleware,validator}/
│   └── migrations/
└── frontend/  # Flutter (Riverpod + Dio + GoRouter)
    └── lib/
        ├── core/{network,router}/
        ├── features/<feature>/{data,domain,presentation}/
        └── shared/widgets/
```

## How it works (ReAct loop, per file)

```
PHP source
  → Generation     : Gemini Flash → Go/Dart code
  → Verification   : go build / dart analyze
      ├─ OK  → extract type signatures → shared_types.json
      └─ ERR → Correction loop:
          ├─ Flash correction #1
          ├─ Flash correction #2
          ├─ Pro escalation
          └─ Failure → status=failed, alert
  → migration_state.json updated (atomic write)
```

Each file gets **at most 4 model calls** (1 generation + 2 Flash corrections + 1 Pro escalation) before being marked `failed`.

## Phases

The scanner classifies Laravel files into 4 sequential phases:

| Phase | Source patterns | Targets |
|---|---|---|
| 1 — Infrastructure | `config/`, `routes/`, `database/migrations/` | Go config + router + SQL migrations |
| 2 — Models | `app/Models/`, `app/Repositories/` | `internal/domain/` + `internal/repository/` |
| 3 — Services / Controllers | `app/Http/Controllers/`, `app/Services/`, `app/Http/Requests/` | `internal/handler/` + `internal/service/` |
| 4 — UI | `resources/views/*.blade.php` | `frontend/lib/features/<feature>/presentation/screens/` |

Phases run **strictly in order**; within a phase, files are processed in parallel by the worker pool.

## Toolchain required on the host

- Go 1.22+
- Dart / Flutter SDK (for `dart analyze` during Phase 4)

## Testing the agent itself

```bash
go test ./internal/...              # unit tests
go test -tags=e2e ./internal/agent/ # E2E with stub Gemini client
```

## License

Internal tool; not yet released publicly.
