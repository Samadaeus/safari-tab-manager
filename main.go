package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "github.com/mattn/go-sqlite3"
)

var (
	titleStyle        = lipgloss.NewStyle().MarginLeft(2)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	duplicateStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	normalStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	oldTabStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // Orange for old tabs
	helpStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

type Tab struct {
	WindowIndex int
	TabIndex    int
	Title       string
	URL         string
	DuplicateOf *int
	Selected    bool
	LastVisit   time.Time
	IsOld       bool // True if last visited > 30 days ago
}

type item struct {
	tab   Tab
	index int
}

func (i item) FilterValue() string { return i.tab.Title }

type itemDelegate struct{}

func (d itemDelegate) Height() int                             { return 3 }
func (d itemDelegate) Spacing() int                            { return 1 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	// Check if this item is currently focused
	isFocused := index == m.Index()
	cursor := "  "
	if isFocused {
		cursor = "→ "
	}

	checkbox := "[ ]"
	if i.tab.Selected {
		checkbox = "[✓]"
	}

	var title string
	var ageIndicator string
	if i.tab.IsOld {
		ageIndicator = " 🕐" // Clock emoji for old tabs
	}

	titleText := fmt.Sprintf("%s%s %s%s", cursor, checkbox, i.tab.Title, ageIndicator)

	if i.tab.DuplicateOf != nil {
		title = duplicateStyle.Render(titleText)
	} else if i.tab.IsOld {
		title = oldTabStyle.Render(titleText)
	} else {
		title = normalStyle.Render(titleText)
	}

	// Add visual emphasis to focused item
	if isFocused {
		title = lipgloss.NewStyle().Bold(true).Render(title)
	}

	urlLine := helpStyle.Render(fmt.Sprintf("    URL: %s", i.tab.URL))

	var duplicateInfo string
	if i.tab.DuplicateOf != nil {
		duplicateInfo = helpStyle.Render(fmt.Sprintf("    → Duplicate of tab #%d", *i.tab.DuplicateOf+1))
	} else {
		infoStr := fmt.Sprintf("    Window %d, Tab %d", i.tab.WindowIndex, i.tab.TabIndex)
		if i.tab.IsOld && !i.tab.LastVisit.IsZero() {
			daysSince := int(time.Since(i.tab.LastVisit).Hours() / 24)
			infoStr += fmt.Sprintf(" • Last visited %d days ago", daysSince)
		}
		duplicateInfo = helpStyle.Render(infoStr)
	}

	fmt.Fprintf(w, "%s\n%s\n%s", title, urlLine, duplicateInfo)
}

type model struct {
	list                  list.Model
	tabs                  []Tab
	quitting              bool
	closing               bool
	ageDays               int // Age threshold in days
	progress              progress.Model
	closingTotal          int
	closingCurrent        int
	closingDone           bool
	message               string
	emptyPinnedOnlyWindows []int // Windows that only contain pinned tabs
}

// Messages for async operations
type tabClosedMsg struct {
	index int
	total int
}

type closingCompleteMsg struct {
	count int
}

type tabsRefreshedMsg struct {
	tabs         []Tab
	emptyWindows []int
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 4)
		return m, nil

	case tabClosedMsg:
		m.closingCurrent = msg.index
		if m.closingCurrent < m.closingTotal {
			return m, nil
		}
		return m, nil

	case closingCompleteMsg:
		m.closingDone = true
		m.message = fmt.Sprintf("Successfully closed %d tabs. Refreshing...", msg.count)
		return m, refreshTabsCmd(m.ageDays)

	case tabsRefreshedMsg:
		m.tabs = msg.tabs
		m.emptyPinnedOnlyWindows = msg.emptyWindows
		m.closing = false
		m.closingDone = false
		m.closingTotal = 0
		m.closingCurrent = 0

		// Update list items
		items := make([]list.Item, len(m.tabs))
		for i, tab := range m.tabs {
			items[i] = item{tab: tab, index: i}
		}
		m.list.SetItems(items)
		m.message = fmt.Sprintf("Tabs refreshed. Press 'q' to quit.")
		return m, nil

	case tea.KeyMsg:
		// Don't accept input while closing
		if m.closing && !m.closingDone {
			return m, nil
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))):
			m.list.CursorDown()
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))):
			m.list.CursorUp()
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys(" ", "enter"))):
			if i, ok := m.list.SelectedItem().(item); ok {
				m.tabs[i.index].Selected = !m.tabs[i.index].Selected
				items := make([]list.Item, len(m.tabs))
				for idx, tab := range m.tabs {
					items[idx] = item{tab: tab, index: idx}
				}
				m.list.SetItems(items)
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			// Collect tabs to close
			tabsToClose := []Tab{}
			for _, tab := range m.tabs {
				if tab.Selected {
					tabsToClose = append(tabsToClose, tab)
				}
			}

			if len(tabsToClose) == 0 {
				m.message = "No tabs selected for closing."
				return m, nil
			}

			m.closing = true
			m.closingTotal = len(tabsToClose)
			m.closingCurrent = 0
			m.closingDone = false
			return m, closeTabsAsync(tabsToClose, m.emptyPinnedOnlyWindows)

		case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
			for i := range m.tabs {
				if m.tabs[i].DuplicateOf != nil {
					m.tabs[i].Selected = true
				}
			}
			items := make([]list.Item, len(m.tabs))
			for idx, tab := range m.tabs {
				items[idx] = item{tab: tab, index: idx}
			}
			m.list.SetItems(items)
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("n"))):
			for i := range m.tabs {
				if m.tabs[i].DuplicateOf != nil {
					m.tabs[i].Selected = false
				}
			}
			items := make([]list.Item, len(m.tabs))
			for idx, tab := range m.tabs {
				items[idx] = item{tab: tab, index: idx}
			}
			m.list.SetItems(items)
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("o"))):
			for i := range m.tabs {
				if m.tabs[i].IsOld {
					m.tabs[i].Selected = true
				}
			}
			items := make([]list.Item, len(m.tabs))
			for idx, tab := range m.tabs {
				items[idx] = item{tab: tab, index: idx}
			}
			m.list.SetItems(items)
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return "Cancelled. No tabs were closed.\n"
	}

	if m.closing {
		var status string
		if m.closingDone {
			status = m.message
		} else {
			percent := float64(m.closingCurrent) / float64(m.closingTotal)
			bar := m.progress.ViewAs(percent)
			status = fmt.Sprintf("Closing tabs... %d/%d\n%s", m.closingCurrent, m.closingTotal, bar)
		}
		return titleStyle.Render(status) + "\n"
	}

	duplicateCount := 0
	uniqueCount := 0
	oldCount := 0
	for _, tab := range m.tabs {
		if tab.DuplicateOf != nil {
			duplicateCount++
		} else {
			uniqueCount++
		}
		if tab.IsOld {
			oldCount++
		}
	}

	selectedCount := 0
	for _, tab := range m.tabs {
		if tab.Selected {
			selectedCount++
		}
	}

	header := titleStyle.Render(fmt.Sprintf(
		"Safari Tab Manager - %d unique, %d duplicates, %d old (>%d days), %d selected to close",
		uniqueCount,
		duplicateCount,
		oldCount,
		m.ageDays,
		selectedCount,
	))

	help := helpStyle.Render(
		"\nk/↑ j/↓: navigate • space/enter: toggle • a: select all duplicates • o: select all old • n: deselect all • c: close selected • q: quit\n",
	)

	var messageDisplay string
	if m.message != "" {
		messageDisplay = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(m.message) + "\n"
	}

	return fmt.Sprintf("%s%s\n\n%s%s", header, messageDisplay, m.list.View(), help)
}

func closeTabsAsync(tabsToClose []Tab, emptyWindows []int) tea.Cmd {
	return func() tea.Msg {
		// Get current Safari state to match tabs by URL
		currentTabs, err := getSafariTabsRaw()
		if err != nil {
			log.Printf("Error getting current tabs: %v", err)
			return closingCompleteMsg{count: 0}
		}

		// Build a set of URLs to close
		urlsToClose := make(map[string]bool)
		for _, tab := range tabsToClose {
			urlsToClose[tab.URL] = true
		}

		// Find matching tabs in current Safari state
		type windowTab struct {
			window int
			tab    int
			url    string
		}

		tabsToCloseNow := []windowTab{}
		for _, tab := range currentTabs {
			if urlsToClose[tab.URL] {
				tabsToCloseNow = append(tabsToCloseNow, windowTab{
					window: tab.WindowIndex,
					tab:    tab.TabIndex,
					url:    tab.URL,
				})
				delete(urlsToClose, tab.URL)
			}
		}

		// Sort by window (desc) and tab index (desc)
		sort.Slice(tabsToCloseNow, func(i, j int) bool {
			if tabsToCloseNow[i].window != tabsToCloseNow[j].window {
				return tabsToCloseNow[i].window > tabsToCloseNow[j].window
			}
			return tabsToCloseNow[i].tab > tabsToCloseNow[j].tab
		})

		// Close tabs one by one
		for idx, wt := range tabsToCloseNow {
			applescript := fmt.Sprintf(`
			tell application "Safari"
				close tab %d of window %d
			end tell
			`, wt.tab, wt.window)

			cmd := exec.Command("osascript", "-e", applescript)
			if err := cmd.Run(); err != nil {
				log.Printf("Warning: failed to close tab %d in window %d: %v", wt.tab, wt.window, err)
			}

			// Send progress update (note: in real bubbletea, we'd use tea.Cmd properly)
			// For now, we'll just close all at once
			_ = idx
		}

		// Close windows that only contained pinned tabs (in descending order)
		sort.Sort(sort.Reverse(sort.IntSlice(emptyWindows)))
		for _, windowIdx := range emptyWindows {
			applescript := fmt.Sprintf(`
			tell application "Safari"
				close window %d
			end tell
			`, windowIdx)

			cmd := exec.Command("osascript", "-e", applescript)
			if err := cmd.Run(); err != nil {
				log.Printf("Warning: failed to close window %d: %v", windowIdx, err)
			}
		}

		return closingCompleteMsg{count: len(tabsToCloseNow)}
	}
}

func refreshTabsCmd(ageDays int) tea.Cmd {
	return func() tea.Msg {
		tabs, emptyWindows, err := getSafariTabs(ageDays)
		if err != nil {
			log.Printf("Error refreshing tabs: %v", err)
			return tabsRefreshedMsg{tabs: []Tab{}, emptyWindows: []int{}}
		}

		tabs = findDuplicates(tabs)
		return tabsRefreshedMsg{tabs: tabs, emptyWindows: emptyWindows}
	}
}

func getSafariTabsRaw() ([]Tab, error) {
	applescript := `
	tell application "Safari"
		set output to ""
		repeat with w from 1 to count of windows
			repeat with t from 1 to count of tabs of window w
				set tabTitle to name of tab t of window w
				set tabURL to URL of tab t of window w
				set output to output & w & "|||" & t & "|||" & tabTitle & "|||" & tabURL & "###"
			end repeat
		end repeat
		return output
	end tell
	`

	cmd := exec.Command("osascript", "-e", applescript)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get Safari tabs: %w", err)
	}

	allTabs := []Tab{}
	lines := strings.Split(strings.TrimSpace(string(output)), "###")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|||")
		if len(parts) != 4 {
			continue
		}

		var windowIndex, tabIndex int
		fmt.Sscanf(parts[0], "%d", &windowIndex)
		fmt.Sscanf(parts[1], "%d", &tabIndex)

		allTabs = append(allTabs, Tab{
			WindowIndex: windowIndex,
			TabIndex:    tabIndex,
			Title:       parts[2],
			URL:         parts[3],
			Selected:    false,
		})
	}

	return allTabs, nil
}

func getSafariTabs(ageDays int) ([]Tab, []int, error) {
	allTabs, err := getSafariTabsRaw()
	if err != nil {
		return nil, nil, err
	}

	// Filter out pinned tabs: tabs that appear at the same early position
	// across multiple windows with the same URL are likely pinned
	tabs, emptyWindows := filterPinnedTabs(allTabs)

	// Enrich tabs with visit history data
	tabs = enrichWithVisitData(tabs, ageDays)

	return tabs, emptyWindows, nil
}

func enrichWithVisitData(tabs []Tab, ageDays int) []Tab {
	// Get Safari history database path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Warning: could not get home directory: %v", err)
		return tabs
	}

	historyPath := filepath.Join(homeDir, "Library", "Safari", "History.db")
	db, err := sql.Open("sqlite3", historyPath)
	if err != nil {
		log.Printf("Warning: could not open Safari history: %v", err)
		return tabs
	}
	defer db.Close()

	// Build map of URL to last visit time
	visitTimes := make(map[string]time.Time)

	query := `
		SELECT hi.url, MAX(hv.visit_time) as last_visit
		FROM history_items hi
		JOIN history_visits hv ON hi.id = hv.history_item
		GROUP BY hi.url
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Warning: could not query Safari history: %v", err)
		return tabs
	}
	defer rows.Close()

	// Safari uses Core Foundation Absolute Time (seconds since Jan 1, 2001)
	// Convert to Unix time by adding the offset
	cfAbsoluteTimeOffset := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC).Unix()

	for rows.Next() {
		var url string
		var visitTime float64
		if err := rows.Scan(&url, &visitTime); err != nil {
			continue
		}

		// Convert CF Absolute Time to Go time
		unixTime := int64(visitTime) + cfAbsoluteTimeOffset
		visitTimes[url] = time.Unix(unixTime, 0)
	}

	// Enrich tabs with visit data
	ageThreshold := time.Now().AddDate(0, 0, -ageDays)

	for i := range tabs {
		if lastVisit, ok := visitTimes[tabs[i].URL]; ok {
			tabs[i].LastVisit = lastVisit
			tabs[i].IsOld = lastVisit.Before(ageThreshold)
		} else {
			// If no visit history, consider it old (never visited or very old)
			tabs[i].IsOld = true
		}
	}

	return tabs
}

func filterPinnedTabs(allTabs []Tab) ([]Tab, []int) {
	// Count how many windows have each URL at low tab indices (1-4)
	urlPositionCount := make(map[string]map[int]int) // url -> tabIndex -> count

	for _, tab := range allTabs {
		if tab.TabIndex <= 4 {
			if urlPositionCount[tab.URL] == nil {
				urlPositionCount[tab.URL] = make(map[int]int)
			}
			urlPositionCount[tab.URL][tab.TabIndex]++
		}
	}

	// Determine which URLs are pinned (appear at same position in 3+ windows)
	pinnedURLs := make(map[string]bool)
	for url, positionCounts := range urlPositionCount {
		for _, count := range positionCounts {
			if count >= 3 {
				pinnedURLs[url] = true
				break
			}
		}
	}

	// Group tabs by window and track pinned tabs per window
	windowTabs := make(map[int][]Tab)
	windowPinnedCount := make(map[int]int)
	windowTotalCount := make(map[int]int)

	for _, tab := range allTabs {
		windowTabs[tab.WindowIndex] = append(windowTabs[tab.WindowIndex], tab)
		windowTotalCount[tab.WindowIndex]++
		if tab.TabIndex <= 4 && pinnedURLs[tab.URL] {
			windowPinnedCount[tab.WindowIndex]++
		}
	}

	// Find windows that only contain pinned tabs
	var emptyWindows []int
	for windowIdx, totalCount := range windowTotalCount {
		pinnedCount := windowPinnedCount[windowIdx]
		if totalCount > 0 && pinnedCount == totalCount {
			emptyWindows = append(emptyWindows, windowIdx)
		}
	}

	// Filter out pinned tabs
	var result []Tab
	for _, tab := range allTabs {
		// Only exclude tabs at early positions that match pinned URLs
		if tab.TabIndex <= 4 && pinnedURLs[tab.URL] {
			continue
		}
		result = append(result, tab)
	}

	return result, emptyWindows
}

func findDuplicates(tabs []Tab) []Tab {
	for i := range tabs {
		for j := 0; j < i; j++ {
			// Exact URL match
			if tabs[i].URL == tabs[j].URL {
				idx := j
				tabs[i].DuplicateOf = &idx
				tabs[i].Selected = true
				break
			}

			// Similar URL (same domain and similar path)
			if areSimilarURLs(tabs[i].URL, tabs[j].URL) {
				idx := j
				tabs[i].DuplicateOf = &idx
				tabs[i].Selected = true
				break
			}
		}
	}

	return tabs
}

func areSimilarURLs(url1, url2 string) bool {
	// Simple similarity check: same domain
	domain1 := extractDomain(url1)
	domain2 := extractDomain(url2)

	if domain1 == "" || domain2 == "" {
		return false
	}

	if domain1 != domain2 {
		return false
	}

	// Check if paths are similar (at least 70% match)
	path1 := extractPath(url1)
	path2 := extractPath(url2)

	if path1 == path2 {
		return true
	}

	similarity := calculateSimilarity(path1, path2)
	return similarity > 0.7
}

func extractDomain(url string) string {
	// Simple domain extraction
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "www.")

	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return strings.ToLower(parts[0])
	}
	return ""
}

func extractPath(url string) string {
	parts := strings.SplitN(url, "//", 2)
	if len(parts) < 2 {
		return ""
	}

	parts = strings.SplitN(parts[1], "/", 2)
	if len(parts) < 2 {
		return ""
	}

	return "/" + strings.TrimSuffix(parts[1], "/")
}

func calculateSimilarity(s1, s2 string) float64 {
	// Levenshtein distance based similarity
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	if s1 == s2 {
		return 1.0
	}

	len1 := len(s1)
	len2 := len(s2)

	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	// Create matrix
	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}

			matrix[i][j] = min(
				matrix[i-1][j]+1,
				matrix[i][j-1]+1,
				matrix[i-1][j-1]+cost,
			)
		}
	}

	distance := matrix[len1][len2]
	maxLen := max(len1, len2)

	return 1.0 - float64(distance)/float64(maxLen)
}

func min(nums ...int) int {
	if len(nums) == 0 {
		return 0
	}
	m := nums[0]
	for _, n := range nums[1:] {
		if n < m {
			m = n
		}
	}
	return m
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}


func main() {
	// Parse command-line flags
	ageDays := flag.Int("age", 30, "Age threshold in days for highlighting old tabs")
	flag.Parse()

	// Validate age
	if *ageDays < 1 {
		fmt.Fprintf(os.Stderr, "Error: age must be at least 1 day\n")
		os.Exit(1)
	}

	tabs, emptyWindows, err := getSafariTabs(*ageDays)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(tabs) == 0 {
		fmt.Println("No Safari tabs found. Is Safari running?")
		os.Exit(0)
	}

	tabs = findDuplicates(tabs)

	// Convert tabs to list items
	items := make([]list.Item, len(tabs))
	for i, tab := range tabs {
		items[i] = item{tab: tab, index: i}
	}

	const defaultWidth = 80
	const listHeight = 20

	l := list.New(items, itemDelegate{}, defaultWidth, listHeight)
	l.Title = "Safari Tabs"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = titleStyle
	l.DisableQuitKeybindings()

	// Disable default list keybindings for navigation
	l.KeyMap.CursorUp.SetEnabled(false)
	l.KeyMap.CursorDown.SetEnabled(false)

	// Initialize progress bar
	prog := progress.New(progress.WithDefaultGradient())

	m := model{list: l, tabs: tabs, ageDays: *ageDays, progress: prog, emptyPinnedOnlyWindows: emptyWindows}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}
