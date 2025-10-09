# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Safari Tab Manager is a macOS-only Terminal User Interface (TUI) application written in Go that helps users find and close duplicate Safari tabs, identify old tabs, and clean up their browsing workspace. The entire application is a single-file Go program (main.go) using the Bubble Tea TUI framework.

## Essential Commands

```bash
# Build the application
go build -o safari-tab-manager

# Run the application
./safari-tab-manager

# Run with custom age threshold (e.g., 60 days)
./safari-tab-manager -age 60

# Print version
./safari-tab-manager -version

# Test releases locally with GoReleaser
goreleaser check
```

## Architecture Overview

### Single-File Design

The entire application is in `main.go` (~827 lines). There are no separate packages or modules. All functionality is implemented as functions and types within the main package.

### Core Components

1. **AppleScript Integration** (main.go:431-495)
   - Uses `osascript` via `exec.Command()` to query Safari tabs
   - Safari automation permissions required (System Settings → Privacy & Security → Automation)
   - All Safari interactions are synchronous shell commands

2. **Safari History Database** (main.go:497-560)
   - Reads `~/Library/Safari/History.db` directly using SQLite
   - Converts Core Foundation Absolute Time (seconds since Jan 1, 2001) to Unix time
   - Uses `modernc.org/sqlite` (pure Go SQLite implementation, CGO_ENABLED=0)

3. **Bubble Tea TUI** (main.go:114-337)
   - Model-View-Update architecture pattern
   - `model` struct contains all application state
   - Messages drive async operations (tabClosedMsg, closingCompleteMsg, tabsRefreshedMsg)

4. **Duplicate Detection** (main.go:621-743)
   - Exact URL matching
   - Domain similarity with Levenshtein distance (>70% threshold)
   - Custom string similarity algorithm for path comparison

5. **Pinned Tab Filtering** (main.go:562-619)
   - Heuristic: tabs at positions 1-4 appearing in 3+ windows are considered pinned
   - Windows containing only pinned tabs are tracked and closed during cleanup

### Key Data Structures

- **Tab** (main.go:35-44): Represents a Safari tab with position, URL, title, visit time, and selection state
- **model** (main.go:114-126): Bubble Tea model containing list state, tabs, progress bar, and async operation tracking
- **item** (main.go:46-48): Wrapper for Tab to implement `list.Item` interface

### Critical Implementation Details

**Tab Closing Order** (main.go:373-379):
- Tabs must be closed in descending order (high window/tab index → low) to prevent index shifting
- Fresh Safari state is fetched immediately before closing to ensure accuracy
- URL-based matching (not index-based) prevents race conditions

**Keyboard Navigation** (main.go:189-271):
- Custom keybindings override default list navigation
- Space/Enter: toggle selection
- 'a': select all duplicates
- 'o': select all old tabs
- 'n': deselect all
- 'c': close selected tabs (was 'Enter' in early versions)
- 'q' or Ctrl+C: quit

**Version Injection**:
- Version is set via ldflags: `-X main.Version={{.Version}}`
- GoReleaser handles this automatically during release builds
- Default version is "dev" for local builds

## macOS-Specific Constraints

- **Platform**: macOS only (requires Safari and AppleScript)
- **Permissions**: Requires automation permissions for the terminal app to control Safari
- **Safari History Path**: Hardcoded to `~/Library/Safari/History.db`
- **CGO**: Disabled (CGO_ENABLED=0) - uses pure Go SQLite implementation

## Release Process

GoReleaser is configured to build universal binaries (amd64 + arm64 merged) for macOS. The GitHub Actions workflow (`.github/workflows/release.yml`) triggers releases when tags are pushed.

Version is injected via ldflags during build: `-X main.Version={{.Version}}`

## Code Patterns

**AppleScript Execution Pattern**:
```go
cmd := exec.Command("osascript", "-e", applescript)
output, err := cmd.Output()
```

**Async Operations in Bubble Tea**:
- Long-running operations (tab closing, refreshing) return `tea.Cmd` functions
- These functions return messages that drive state transitions
- Progress is tracked via `closingCurrent` and `closingTotal` counters

**Database Query Pattern**:
```go
db, err := sql.Open("sqlite3", historyPath)
defer db.Close()
rows, err := db.Query(query)
defer rows.Close()
```
