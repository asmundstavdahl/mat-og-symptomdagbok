# Mat- og Symptomdagbok

Dette er et Go-program for å registrere måltider og symptomer.

## For utviklere

1. Kjør `make init` for å:
   - Opprette midlertidig mappe (`.tmp`)
   - Konfigurere Git pre-commit hook
2. Kjør `make run` for å starte programmet (TMPDIR er satt til `.tmp`).

## Git pre-commit hook

Pre-commit hook-en ligger i `.githooks/pre-commit` (Makefile init kjører `git config core.hooksPath .githooks`). Hook-en kjører følgende sjekker:

- Whitespace-sjekk
- `gofmt` (formaterer automatisk Go-kode og legger endringer til staging area)
- `go vet`
- `go mod tidy` (sjekker at `go.mod` og `go.sum` er oppdatert)
- `go build ./...`
- `go test ./...`