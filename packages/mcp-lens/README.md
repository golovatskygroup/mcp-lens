# @golovatskygroup/mcp-lens

A lightweight NPX wrapper for [mcp-lens](https://github.com/golovatskygroup/mcp-lens) that automatically:

- Detects your OS and CPU architecture
- Downloads the appropriate binary from GitHub releases
- Verifies the SHA256 checksum for security
- Caches the binary locally in `~/.cache/mcp-lens/`
- Spawns the binary with all arguments forwarded

## Installation

```bash
npm install -g @golovatskygroup/mcp-lens
```

Or use it directly via `npx`:

```bash
npx @golovatskygroup/mcp-lens [options]
```

## Usage

Once installed, you can run `mcp-lens` directly:

```bash
mcp-lens -config ./config.yaml
```

Or with npx:

```bash
npx @golovatskygroup/mcp-lens -config ./config.yaml
```

## How it Works

1. **Platform Detection**: Determines your OS (`darwin`, `linux`, `windows`) and architecture (`arm64`, `amd64`).
2. **Version Mapping**: Uses the package version (e.g., `0.1.0`) to derive the GitHub release tag (e.g., `v0.1.0`).
3. **Download & Verify**: Downloads the binary and `checksums.txt` from the GitHub release, then verifies the SHA256 hash.
4. **Local Caching**: Stores the binary in `~/.cache/mcp-lens/` to avoid re-downloading on subsequent runs.
5. **Execution**: Spawns the binary with `stdio: inherit` so all output and input is passed through seamlessly.

## Environment Variables

None required by the wrapper itself, but `mcp-lens` respects:

- `GITHUB_TOKEN` — Passed to the upstream GitHub MCP server for authentication.

## Supported Platforms

- macOS (Apple Silicon & Intel): `darwin_arm64`, `darwin_amd64`
- Linux (x86_64 & ARM64): `linux_amd64`, `linux_arm64`
- Windows (x86_64): `windows_amd64`

## Cache Location

Binaries are cached in:

```
~/.cache/mcp-lens/
```

Clear the cache manually if needed:

```bash
rm -rf ~/.cache/mcp-lens/
```

## License

MIT — see the main repository LICENSE.

## Contributing

Issues and PRs welcome at [golovatskygroup/mcp-proxy](https://github.com/golovatskygroup/mcp-proxy).
