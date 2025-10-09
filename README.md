# Safari Tab Manager

A Terminal User Interface (TUI) for macOS that helps you find and close duplicate Safari tabs, identify old tabs, and clean up your browsing workspace.

## Features

- 🔍 Enumerates all Safari tabs across all windows
- 🔄 Identifies exact and semi-related duplicate tabs
- 📌 Automatically filters out pinned tabs
- 🕐 Detects and highlights tabs older than a configurable threshold (default: 30 days)
- ✅ Interactive selection with checkboxes
- 🎨 Beautiful TUI with color-coded duplicates and old tabs
- ⌨️  Keyboard navigation with arrow keys and vim bindings (j/k)
- 📊 Shows counts for unique tabs, duplicates, old tabs, and selected tabs
- 🚀 Fast and lightweight
- 📈 Progress bar during tab closing operations
- 🔄 Auto-refresh after closing tabs
- 🪟 Automatically closes windows that only contain pinned tabs

## Requirements

- macOS (uses AppleScript to interact with Safari)
- Go 1.21 or higher
- Safari browser

## Installation

```bash
# Clone or download the files
cd safari-tab-manager

# Download dependencies
go mod download

# Build the application
go build -o safari-tab-manager

# Make it executable
chmod +x safari-tab-manager

# Optionally, move to PATH
sudo mv safari-tab-manager /usr/local/bin/
```

## Usage

Simply run the application:

```bash
./safari-tab-manager
```

Or if installed to PATH:

```bash
safari-tab-manager
```

### Command-line Options

- **-age N** - Set the age threshold in days for highlighting old tabs (default: 30)

Example:

```bash
# Highlight tabs older than 60 days
./safari-tab-manager -age 60
```

### Keyboard Controls

- **↑/↓** or **j/k** - Navigate through tabs (focused tab shown with → cursor)
- **Space** - Toggle selection for current tab
- **Enter** - Close selected tabs (shows progress bar and auto-refreshes)
- **a** - Select all duplicate tabs
- **o** - Select all old tabs (based on age threshold)
- **n** - Deselect all tabs
- **q** or **Ctrl+C** - Quit the application

## How It Works

The application:

1. Uses AppleScript to query Safari for all open tabs across all windows
2. Automatically detects and filters out pinned tabs (tabs at positions 1-4 appearing in 3+ windows)
3. Queries Safari's History.db to determine when each tab was last visited
4. Analyzes URLs and titles to identify duplicates:
   - **Exact duplicates**: Same URL
   - **Similar duplicates**: Same domain with similar paths (>70% similarity)
5. Displays results in an interactive TUI with color coding and visual indicators
6. Shows status bar with counts: unique tabs, duplicates, old tabs, and selected tabs
7. Pre-selects duplicate tabs for closing
8. Allows you to review and toggle selections
9. When you press Enter:
   - Shows a progress bar during tab closing
   - Closes selected tabs (sorted to prevent index shifts)
   - Closes windows that only contained pinned tabs
   - Auto-refreshes the tab list

## Duplicate Detection

The app identifies duplicates based on:

- **Exact URL matches**: Tabs with identical URLs
- **Domain similarity**: Tabs from the same domain with similar paths
- **Path similarity**: Uses Levenshtein distance (>70% threshold)

Examples of detected duplicates:
- `https://github.com/user/repo` and `https://github.com/user/repo/`
- `https://example.com/article` and `https://www.example.com/article`
- `https://site.com/page?id=1` and `https://site.com/page?id=2`

## Old Tab Detection

The app identifies tabs that haven't been visited recently by:

1. Reading Safari's History.db (located at `~/Library/Safari/History.db`)
2. Finding the last visit timestamp for each tab's URL
3. Converting Safari's Core Foundation Absolute Time to standard timestamps
4. Comparing against the age threshold (configurable via `-age` flag, default: 30 days)

Old tabs are displayed in **orange** with a **🕐** emoji indicator. Use the **o** key to quickly select all old tabs for closing.

## Pinned Tab Handling

The app automatically detects pinned tabs using pattern analysis:
- Tabs at positions 1-4 in the tab bar
- That appear with the same URL in 3 or more windows

These tabs are filtered out and never shown in the list. If a window only contains pinned tabs (after filtering), the entire window will be closed during the cleanup operation.

## Display

The TUI shows:

**Status Bar** (at the top):
```
N unique, M duplicates, X old (>30 days), Y selected to close
```

**Tab List** with visual indicators:

- **→** cursor shows the currently focused tab (bold text)
- **[✓]** or **[ ]** checkbox for selection
- **Red color** for duplicate tabs
- **Orange color + 🕐** for old tabs (last visited beyond age threshold)

Example display:

```
→ [✓] Article Title (DUPLICATE) 🕐
      URL: https://example.com/article
      → Duplicate of tab #5

  [ ] Original Article Title
      URL: https://example.com/article
      Window 1, Tab 5
```

## Permissions

On first run, macOS may ask for permission to control Safari. You'll need to grant this permission in:

**System Settings → Privacy & Security → Automation → Terminal** (or your terminal app)

And check the box for Safari.

## Notes

- **Pinned tabs** are automatically filtered out and never shown in the list
- **Windows with only pinned tabs** are automatically closed during cleanup
- Tabs are closed in descending order (by window and tab index) to prevent index shifting
- The app fetches fresh Safari state before closing to ensure accuracy
- A **progress bar** shows the closing operation in real-time
- The tab list **auto-refreshes** after closing so you can continue working
- No tabs are closed until you explicitly press **Enter**
- You can quit safely at any time with **q** without closing any tabs
- Tab age is determined from Safari's History.db (last visit timestamp)

## Troubleshooting

**"Failed to get Safari tabs"**: 
- Make sure Safari is running
- Check that automation permissions are granted

**Tabs not closing**:
- Verify Safari automation permissions
- Some protected tabs may not be closeable via AppleScript

## License

MIT License - feel free to use and modify as needed.
