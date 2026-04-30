# Git hooks

`pre-commit` mirrors `.github/workflows/ci.yml`: `gofmt`, `go vet`, `go build`, `go test -race`, `golangci-lint`, `govulncheck`. Catches the same issues CI does, locally, before push.

## One-time activation

```bash
git config core.hooksPath .githooks
```

The hook is checked into the repo; this just tells your local clone to use `.githooks/` instead of `.git/hooks/`.

## Required tools

`gofmt`, `go vet`, `go build`, `go test` ship with Go. The other two need installing once:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.1
go install golang.org/x/vuln/cmd/govulncheck@latest
```

The hook prints a yellow warning and continues if either is missing — CI will then catch what the hook couldn't.

## Bypass

For WIP commits to a personal branch where you knowingly accept the lint debt:

```bash
git commit --no-verify
```

Avoid on `main`.
