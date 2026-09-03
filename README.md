<div align="center">

# tempo

A [Temporal](https://temporal.io) TUI that matches your rhythm

[![GitHub Release](https://img.shields.io/github/v/release/galaxy-io/tempo)](https://github.com/galaxy-io/tempo/releases)
[![License](https://img.shields.io/github/license/galaxy-io/tempo)](https://github.com/galaxy-io/tempo/blob/main/LICENSE)
![GitHub Downloads](https://img.shields.io/github/downloads/galaxy-io/tempo/total)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/galaxy-io/tempo)

</div>

<p align="center">
  <img src="./assets/tempo-demo.gif" alt="Tempo Demo" width="800">
</p>

## Features

**Workflow Management**

- Browse workflows across namespaces
- View workflow details, inputs, outputs, and metadata
- Inspect full event history with tree and timeline views
- Cancel, terminate, or signal running workflows
- Advanced search with visibility queries and saved filters

**Namespace Operations**

- List and browse all namespaces
- View namespace configuration and details
- Quick namespace switching

**Task Queues & Schedules**

- Monitor task queue activity
- View and manage schedules

**Connection Profiles**

- Save multiple Temporal server configurations
- TLS/mTLS support with certificate paths
- Quick profile switching with `P` key

**Customization**

- 26 built-in color themes (dark and light variants)
- Themes include: TokyoNight, Catppuccin, Dracula, Nord, Gruvbox, One Dark, Solarized, Rosé Pine, Kanagawa, Everforest, Monokai, GitHub
- Live theme preview while selecting

## Installation

### From Source

```bash
go install github.com/galaxy-io/tempo/cmd/tempo@latest
```

### Brew

Brew installed versions will not recieve auto-updates you must update with `brew update`

```bash
brew install galaxy-io/tap/tempo
```

### Build Locally

```bash
git clone https://github.com/galaxy-io/tempo.git
cd tempo
go build -o tempo ./cmd/tempo
```

## Usage

```bash
tempo --address localhost:7233 // default dev server address loads without flag
```

### Command Line Flags

| Flag                | Description                           |
| ------------------- | ------------------------------------- |
| `--address`         | Temporal server address (host:port)   |
| `--namespace`       | Default namespace                     |
| `--profile`         | Connection profile name (from config) |
| `--tls-cert`        | Path to TLS certificate               |
| `--tls-key`         | Path to TLS private key               |
| `--tls-ca`          | Path to CA certificate                |
| `--tls-server-name` | Server name for TLS verification      |
| `--tls-skip-verify` | Skip TLS verification (insecure)      |
| `--codec-endpoint`  | Codec server URL for decoding payloads |
| `--theme`           | Theme name                            |

### Keybindings

**Navigation**
| Key | Action |
|-----|--------|
| `j` / `k` | Navigate down / up |
| `Enter` | Select / expand |
| `Esc` / `Backspace` | Go back |
| `q` | Quit (from root view) |

**Global**
| Key | Action |
|-----|--------|
| `?` | Show help |
| `T` | Theme selector |
| `P` | Profile selector |
| `:` | Command mode |
| `Ctrl+O` | Toggle mouse support (off allows terminal text selection) |
| `/` | Filter (in workflow list) |

**Workflow Actions**
| Key | Action |
|-----|--------|
| `c` | Cancel workflow |
| `t` | Terminate workflow |
| `s` | Signal workflow |
| `u` | Open workflow in the Temporal Web UI |

## Configuration

Configuration is stored in `~/.config/tempo/config.yaml` (or `$XDG_CONFIG_HOME/tempo/config.yaml`).

```yaml
theme: tokyonight-night
active_profile: local

profiles:
  local:
    address: localhost:7233
    namespace: default

  staging:
    address: temporal.staging.example.com:7233
    namespace: staging
    codec_endpoint: https://codec.example.com
    ui_url: https://temporal.staging.example.com
    tls:
      cert: /path/to/client.pem
      key: /path/to/client-key.pem
      ca: /path/to/ca.pem
```

Auto-refresh (`a` on workflows and namespaces) reloads every second by default.
Set `refresh_rate` to a Go duration (`500ms`, `2s`) or a bare number of seconds:

```yaml
refresh_rate: 1s
workflow_page_size: 100
```

`workflow_page_size` is how many workflows each list page fetches (default 100, between 10 and 1000). Scrolling loads more pages.

The workers tab decides whether an instance is still alive from how recently it
was last seen, because Temporal keeps listing a worker for minutes after it
stops. Both allowances can be tuned:

```yaml
worker_poll_quiet_after: 90s      # task queue poll registry entry unchanged
worker_heartbeat_quiet_after: 3m  # worker heartbeat unchanged, when nothing is polling
```

Past those, the instance reads as `Stale` instead of `Running` or `Polling`.
Values are Go durations, clamped between 5s and 1h.

`codec_endpoint` is the Temporal codec server base URL (the same value as `temporal --codec-endpoint`). Tempo POSTs to `/encode` and `/decode`, sends `X-Namespace`, and substitutes `{namespace}` in the URL when present. Environment variables are expanded (`${TEMPORAL_CODEC_URL}`).

## Themes

<p align="center">
  <img src="./assets/tempo-themeselect.gif" alt="Theme Selection" width="800">
</p>

26 themes are available, organized by color scheme family:

**Dark Themes**

- `tokyonight-night`, `tokyonight-storm`, `tokyonight-moon`
- `catppuccin-mocha`, `catppuccin-macchiato`, `catppuccin-frappe`
- `dracula`
- `nord`
- `gruvbox-dark`
- `onedark`
- `solarized-dark`
- `rosepine`, `rosepine-moon`
- `kanagawa`
- `everforest-dark`
- `monokai`
- `github-dark`

**Light Themes**

- `tokyonight-day`
- `catppuccin-latte`
- `dracula-light`
- `gruvbox-light`
- `onelight`
- `solarized-light`
- `rosepine-dawn`
- `everforest-light`
- `github-light`

Press `T` to open the theme selector with live preview.

## Requirements

- Go 1.21+
- A running Temporal server

MIT License - see [LICENSE](LICENSE) for details.

## Ways to Contribute

You can contribute by:

- reporting bugs
- Proposing features or design improvements
- Improving documentation
- Fixing issues labeled good first issue

To report a bug, make a feature request, and more, visit our issues page

## Acknowledgments

- [Temporal](https://temporal.io) - The workflow engine this client connects to
- [tview](https://github.com/rivo/tview) - Terminal UI library
- [jig](https://github.com/atterpac/jig) - UI component framework
