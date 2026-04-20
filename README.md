# sftp-tui

A terminal UI SFTP client built with Go and [Bubbletea](https://github.com/charmbracelet/bubbletea). Ships as a single static binary.

## Features

- Dual-pane file browser (local + remote)
- Encrypted address book (AES-256-GCM + Argon2id)
- Multi-file select, transfer, delete, mkdir
- Legacy SSH cipher/KEX support for old embedded and enterprise servers
- Tab-based multi-connection interface
- File filtering, sorting, and hidden file toggle

## Install

```sh
export PATH=/usr/lib/go-1.24/bin:$PATH
CGO_ENABLED=0 go build -ldflags="-s -w" -o sftp-tui ./...
```

## Usage

```sh
./sftp-tui                        # interactive mode
./sftp-tui --connect user@host:22 # connect immediately
```

## Key Bindings

### Global

| Key | Action |
|-----|--------|
| `Ctrl+N` | New connection |
| `Ctrl+B` | Address book |
| `Ctrl+W` | Close tab |
| `Alt+1..9` | Switch to tab N |
| `?` | Toggle help |
| `Ctrl+C` | Quit |

### File Pane

| Key | Action |
|-----|--------|
| `↑↓` / `k j` | Navigate |
| `Enter` | Enter directory |
| `Backspace` | Go up |
| `Space` | Toggle multi-select |
| `*` | Select / deselect all |
| `/` | Filter mode |
| `Esc` | Clear filter / cancel |
| `s` | Cycle sort (name → size → date) |
| `h` | Toggle hidden files |
| `r` / `F10` | Refresh |

### Transfer (per-connection tab)

| Key | Action |
|-----|--------|
| `Tab` | Switch pane focus |
| `F5` | Copy (upload or download based on focus) |
| `F6` | Move (copy + delete source) |
| `F7` | Make directory |
| `F8` | Delete selected (with confirmation) |

## Address Book

Entries are stored in `~/.config/sftp-tui/addressbook.enc`.

- Encrypted with AES-256-GCM; key derived via Argon2id (64 MiB, 3 iterations)
- Master password is held in memory only and zeroed on lock or exit
- Each save regenerates a fresh random salt and nonce
- Wrong password produces a GCM tag mismatch — no information is leaked

## SSH Compatibility

Supports legacy servers alongside modern ones:

- **Ciphers:** aes-gcm, chacha20, aes-ctr, aes128-cbc, 3des-cbc, arcfour\*
- **KEX:** curve25519, ecdh-nistp\*, dh-group14-sha256, dh-group14-sha1, dh-group1-sha1
- **MACs:** hmac-sha2-256-etm, hmac-sha2-256, hmac-sha1, hmac-sha1-96
- **Host keys:** ed25519, ecdsa, rsa-sha2-\*, ssh-rsa

> **Note:** Host key verification is currently disabled (`InsecureIgnoreHostKey`). Known-hosts support is planned.

## Testing

```sh
export PATH=/usr/lib/go-1.24/bin:$PATH
go test ./...
```

## Known Limitations

- No `known_hosts` verification — planned future feature
- No recursive directory transfers (selected files only)
- Transfer progress is coarse (start/done, no per-chunk updates)
- `F2` rename is not yet implemented
- `mkdir` creates a hardcoded `new_folder` name — a prompt is a TODO
- Argon2id unlock takes ~0.5s by design (brute-force resistance)
