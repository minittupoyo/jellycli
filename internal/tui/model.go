// Package tui implements the Bubble Tea frontend. It talks only to application
// services and never constructs Jellyfin or mpv requests itself.
package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"jellycli/internal/application"
	"jellycli/internal/jellyfin"
)

type Service interface {
	Home(context.Context) (application.HomeContent, error)
	Libraries(context.Context) ([]jellyfin.Item, error)
	Browse(context.Context, string, []jellyfin.ItemKind) ([]jellyfin.Item, error)
	Search(context.Context, string) ([]jellyfin.Item, error)
	Play(context.Context, string) error
}

type page struct {
	title  string
	rows   []row
	cursor int
}

type row struct {
	label  string
	action action
	item   jellyfin.Item
}

type action uint8

const (
	actionNone action = iota
	actionLibraries
	actionOpen
	actionPlay
)

type Model struct {
	ctx       context.Context
	service   Service
	page      page
	stack     []page
	loading   bool
	searching bool
	query     string
	status    string
	width     int
	height    int
	requestID uint64
}

type loadedMsg struct {
	id   uint64
	page page
	err  error
}

type homeMsg struct {
	id      uint64
	content application.HomeContent
	err     error
}

type playbackFinishedMsg struct{ err error }

func New(ctx context.Context, service Service) Model {
	return Model{ctx: ctx, service: service, page: page{title: "Home"}, loading: true, requestID: 1}
}

func (m Model) Init() tea.Cmd { return m.loadHome(m.requestID) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case homeMsg:
		if msg.id != m.requestID {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.page = homePage(msg.content)
	case loadedMsg:
		if msg.id != m.requestID {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.page = msg.page
	case playbackFinishedMsg:
		if msg.err != nil {
			m.status = "Playback failed: " + msg.err.Error()
		} else {
			m.status = "Playback finished."
		}
	case tea.KeyPressMsg:
		if m.searching {
			return m.updateSearchInput(msg)
		}
		return m.updateNavigation(msg)
	}
	return m, nil
}

func (m Model) updateNavigation(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "/":
		m.searching, m.query, m.status = true, "", ""
	case "up", "k":
		m.page.cursor = previousSelectable(m.page.rows, m.page.cursor)
	case "down", "j":
		m.page.cursor = nextSelectable(m.page.rows, m.page.cursor)
	case "esc":
		if len(m.stack) > 0 && !m.loading {
			m.page = m.stack[len(m.stack)-1]
			m.stack = m.stack[:len(m.stack)-1]
			m.status = ""
		}
	case "enter":
		return m.activate()
	}
	return m, nil
}

func (m Model) updateSearchInput(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.searching, m.query = false, ""
	case "enter":
		term := strings.TrimSpace(m.query)
		if term == "" {
			m.status = "Enter a search term."
			return m, nil
		}
		m.searching = false
		return m.beginLoad("Search: "+term, func(ctx context.Context) ([]jellyfin.Item, error) {
			return m.service.Search(ctx, term)
		})
	case "backspace", "ctrl+h":
		if len(m.query) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.query)
			m.query = m.query[:len(m.query)-size]
		}
	default:
		if key.Key().Text != "" && key.Key().Mod == 0 {
			m.query += key.Key().Text
		}
	}
	return m, nil
}

func (m Model) activate() (tea.Model, tea.Cmd) {
	if m.loading || len(m.page.rows) == 0 {
		return m, nil
	}
	selected := m.page.rows[m.page.cursor]
	switch selected.action {
	case actionLibraries:
		return m.beginLoad("Libraries", m.service.Libraries)
	case actionPlay:
		m.status = "Playing " + selected.item.Name + "…"
		return m, tea.Exec(playCommand{ctx: m.ctx, service: m.service, itemID: selected.item.ID}, func(err error) tea.Msg {
			return playbackFinishedMsg{err: err}
		})
	case actionOpen:
		title, kinds := childQuery(selected.item)
		return m.beginLoad(title, func(ctx context.Context) ([]jellyfin.Item, error) {
			return m.service.Browse(ctx, selected.item.ID, kinds)
		})
	}
	return m, nil
}

func (m Model) beginLoad(title string, load func(context.Context) ([]jellyfin.Item, error)) (tea.Model, tea.Cmd) {
	m.stack = append(m.stack, m.page)
	m.page = page{title: title}
	m.loading, m.status = true, ""
	m.requestID++
	id := m.requestID
	return m, func() tea.Msg {
		items, err := load(m.ctx)
		return loadedMsg{id: id, page: itemPage(title, items), err: err}
	}
}

func (m Model) loadHome(id uint64) tea.Cmd {
	return func() tea.Msg {
		content, err := m.service.Home(m.ctx)
		return homeMsg{id: id, content: content, err: err}
	}
}

func (m Model) View() tea.View {
	var b strings.Builder
	fmt.Fprintf(&b, "jellycli — %s\n\n", m.page.title)
	if m.searching {
		fmt.Fprintf(&b, "Search: %s█\n\n", m.query)
	}
	if m.loading {
		b.WriteString("Loading…\n")
	} else if len(m.page.rows) == 0 {
		b.WriteString("No items found.\n")
	} else {
		limit := len(m.page.rows)
		if m.height > 7 && limit > m.height-6 {
			limit = m.height - 6
		}
		start := 0
		if m.page.cursor >= limit {
			start = m.page.cursor - limit + 1
		}
		for i := start; i < start+limit; i++ {
			marker := "  "
			if i == m.page.cursor {
				marker = "> "
			}
			fmt.Fprintf(&b, "%s%s\n", marker, m.page.rows[i].label)
		}
	}
	if m.status != "" {
		fmt.Fprintf(&b, "\n%s\n", m.status)
	}
	b.WriteString("\n↑/k ↓/j move  Enter open/play  Esc back  / search  q quit")
	view := tea.NewView(b.String())
	view.AltScreen = true
	view.WindowTitle = "jellycli"
	return view
}

func homePage(content application.HomeContent) page {
	rows := []row{{label: "Libraries", action: actionLibraries}}
	rows = appendSection(rows, "Continue Watching", content.ContinueWatching)
	rows = appendSection(rows, "Next Up", content.NextUp)
	rows = appendSection(rows, "Recently Added", content.RecentlyAdded)
	return page{title: "Home", rows: rows}
}

func appendSection(rows []row, title string, items []jellyfin.Item) []row {
	if len(items) == 0 {
		return rows
	}
	rows = append(rows, row{label: "── " + title + " ──"})
	for _, item := range items {
		rows = append(rows, itemRow(item))
	}
	return rows
}

func itemPage(title string, items []jellyfin.Item) page {
	rows := make([]row, 0, len(items))
	for _, item := range items {
		rows = append(rows, itemRow(item))
	}
	return page{title: title, rows: rows}
}

func nextSelectable(rows []row, cursor int) int {
	for i := cursor + 1; i < len(rows); i++ {
		if rows[i].action != actionNone {
			return i
		}
	}
	return cursor
}

func previousSelectable(rows []row, cursor int) int {
	for i := cursor - 1; i >= 0; i-- {
		if rows[i].action != actionNone {
			return i
		}
	}
	return cursor
}

func itemRow(item jellyfin.Item) row {
	label := item.Name
	if item.Type == jellyfin.ItemKindEpisode && item.ParentIndexNumber != nil && item.IndexNumber != nil {
		label = fmt.Sprintf("S%02dE%02d  %s", *item.ParentIndexNumber, *item.IndexNumber, item.Name)
	}
	if item.UserData != nil {
		if item.UserData.Played {
			label += "  ✓"
		} else if item.UserData.PlayedPercentage != nil {
			label += fmt.Sprintf("  %.0f%%", *item.UserData.PlayedPercentage)
		}
	}
	if item.RunTimeTicks != nil && *item.RunTimeTicks > 0 {
		runtime := time.Duration(*item.RunTimeTicks/10_000_000) * time.Second
		label += "  " + runtime.Round(time.Minute).String()
	}
	switch item.Type {
	case jellyfin.ItemKindMovie, jellyfin.ItemKindEpisode, jellyfin.ItemKindVideo:
		return row{label: label, action: actionPlay, item: item}
	default:
		return row{label: label, action: actionOpen, item: item}
	}
}

func childQuery(item jellyfin.Item) (string, []jellyfin.ItemKind) {
	switch item.Type {
	case jellyfin.ItemKindSeries:
		return item.Name, []jellyfin.ItemKind{jellyfin.ItemKindSeason}
	case jellyfin.ItemKindSeason:
		return item.Name, []jellyfin.ItemKind{jellyfin.ItemKindEpisode}
	default:
		if item.CollectionType == "tvshows" {
			return item.Name, []jellyfin.ItemKind{jellyfin.ItemKindSeries}
		}
		if item.CollectionType == "movies" {
			return item.Name, []jellyfin.ItemKind{jellyfin.ItemKindMovie}
		}
		return item.Name, []jellyfin.ItemKind{jellyfin.ItemKindMovie, jellyfin.ItemKindSeries, jellyfin.ItemKindVideo}
	}
}

type playCommand struct {
	ctx     context.Context
	service Service
	itemID  string
}

func (c playCommand) Run() error        { return c.service.Play(c.ctx, c.itemID) }
func (playCommand) SetStdin(io.Reader)  {}
func (playCommand) SetStdout(io.Writer) {}
func (playCommand) SetStderr(io.Writer) {}

func Run(ctx context.Context, service Service) error {
	if service == nil {
		return fmt.Errorf("run TUI: service is required")
	}
	_, err := tea.NewProgram(New(ctx, service), tea.WithContext(ctx)).Run()
	return err
}
