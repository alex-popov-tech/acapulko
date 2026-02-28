# Acapulko

Power outages tracking web app for a specific address. Monitors grid power status via Home Assistant and emergency outage info via DTEK, serves a dashboard.

## Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- [Task](https://taskfile.dev/) — task runner
- [Air](https://github.com/air-verse/air) — hot reload
- [golangci-lint v2](https://golangci-lint.run/) — linter
- [Lefthook](https://github.com/evilmartians/lefthook) — git hooks
- [govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) — vulnerability scanner

Install all tools:

```bash
go install github.com/go-task/task/v3/cmd/task@latest
go install github.com/air-verse/air@latest
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
go install github.com/evilmartians/lefthook@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
```

## Quick Start

```bash
cp .env.example .env   # fill in your secrets
lefthook install        # set up git hooks
task dev                # start with hot reload
```

## Available Commands

| Command | Description |
|---------|-------------|
| `task dev` | Start dev server with hot reload (Air) |
| `task build` | Build binary for current platform → `bin/acapulko` |
| `task build-pi` | Cross-compile for Raspberry Pi 5 (linux/arm64) → `bin/acapulko-linux-arm64` |
| `task lint` | Run golangci-lint |
| `task fmt` | Run code formatters (gofmt + goimports) |
| `task test` | Run tests with race detector |
| `task vuln` | Run govulncheck for known vulnerabilities |
| `task check` | Full quality gate: lint → vuln → test |
| `task clean` | Remove build artifacts |

## Configuration

Copy `.env.example` to `.env` and fill in the values:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PORT` | no | `:8080` | Server listen address |
| `HA_BASE_URL` | yes | — | Home Assistant base URL |
| `HA_TOKEN` | yes | — | Home Assistant long-lived access token |
| `HA_ENTITY` | no | `binary_sensor.inverter_grid` | HA entity to monitor |
| `HA_POLL_INTERVAL` | no | `10s` | How often to poll HA |
| `DTEK_BASE_URL` | yes | — | DTEK API base URL |
| `DTEK_REGION` | no | `oem` | DTEK region code |
| `DTEK_CITY` | yes | — | City name (URL-encoded) |
| `DTEK_STREET` | yes | — | Street name (URL-encoded) |
| `DTEK_POLL_INTERVAL` | no | `60s` | How often to poll DTEK |
| `HISTORY_FILE_PATH` | no | `history.json` | Path to history JSON file |
| `HISTORY_WINDOW` | no | `168h` | Rolling history window (7 days) |
| `SENTRY_DSN` | no | — | Sentry DSN (optional, for error tracking) |
| `SENTRY_ENV` | no | `production` | Sentry environment name |

## Deployment

### Build for Raspberry Pi 5

```bash
task build-pi
# produces bin/acapulko-linux-arm64
scp bin/acapulko-linux-arm64 pi@your-pi:/opt/acapulko/acapulko
```

### Systemd unit file

```ini
[Unit]
Description=Acapulko Power Outages Tracker
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/opt/acapulko/acapulko
WorkingDirectory=/opt/acapulko
EnvironmentFile=/opt/acapulko/.env
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo cp acapulko.service /etc/systemd/system/
sudo systemctl enable acapulko
sudo systemctl start acapulko
sudo journalctl -u acapulko -f   # view logs
```

### Cloudflare DNS

Point `acapulko.oleksandrp.com` to the Pi's IP (or tunnel).

## Releasing

Push a version tag to trigger an automatic GitHub Release with the arm64 binary:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The release workflow builds `acapulko-linux-arm64` and attaches it to the GitHub Release.
