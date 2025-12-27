# CI/CD and Release Review: MCP Router Scope

This document outlines the findings and required changes for the CI/CD pipelines and release configurations to support the transition to a dedicated MCP Router.

## 1. GoReleaser Configuration (`.goreleaser.yaml`)

### Current Risks
- **Over-packaging**: The current config may be capturing all sub-packages in the `cmd/` directory.
- **Large Artifacts**: Inclusion of legacy assets or documentation that increases context size and download time.
- **Platform Bloat**: Building for too many architectures (e.g., mobile) that aren't relevant for a router.

### Required Changes
- **Restrict Builds**: Modify the `builds` section to target only `cmd/mcp-router/main.go`.
- **Exclude Worktrees**: Explicitly exclude `.worktrees/`, `stage*/`, and `task*` directories from the `nfpms` or `archives` sections.
- **Binary Naming**: Ensure the binary is named `mcp-router` (or `mcp-router.exe` on Windows).