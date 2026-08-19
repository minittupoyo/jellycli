package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"jellycli/internal/application"
	"jellycli/internal/jellyfin"
)

type fakeService struct {
	home       application.HomeContent
	libraries  []jellyfin.Item
	browse     []jellyfin.Item
	search     []jellyfin.Item
	parentID   string
	kinds      []jellyfin.ItemKind
	searchTerm string
	playedID   string
	playErr    error
}

func (f *fakeService) Home(context.Context) (application.HomeContent, error) {
	return f.home, nil
}
func (f *fakeService) Libraries(context.Context) ([]jellyfin.Item, error) {
	return f.libraries, nil
}
func (f *fakeService) Browse(_ context.Context, parent string, kinds []jellyfin.ItemKind) ([]jellyfin.Item, error) {
	f.parentID = parent
	f.kinds = append([]jellyfin.ItemKind(nil), kinds...)
	return f.browse, nil
}
func (f *fakeService) Search(_ context.Context, term string) ([]jellyfin.Item, error) {
	f.searchTerm = term
	return f.search, nil
}
func (f *fakeService) Play(_ context.Context, id string) error {
	f.playedID = id
	return f.playErr
}

func key(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Text: text})
}

func update(t *testing.T, model Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := model.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("model type = %T", next)
	}
	return got, cmd
}

func TestHomeLoadsAndNavigationSkipsSectionHeaders(t *testing.T) {
	service := &fakeService{home: application.HomeContent{ContinueWatching: []jellyfin.Item{{ID: "movie", Name: "Movie", Type: jellyfin.ItemKindMovie}}}}
	model := New(context.Background(), service)
	msg := model.Init()()
	model, _ = update(t, model, msg)
	if model.loading || len(model.page.rows) != 3 || !strings.Contains(model.View().Content, "Continue Watching") {
		t.Fatalf("home = %#v, view = %q", model.page, model.View().Content)
	}
	model, _ = update(t, model, key(tea.KeyDown, ""))
	if model.page.cursor != 2 {
		t.Fatalf("cursor = %d, want playable row 2", model.page.cursor)
	}
}

func TestLibrarySeriesSeasonNavigationAndBack(t *testing.T) {
	service := &fakeService{
		libraries: []jellyfin.Item{{ID: "tv", Name: "TV", Type: jellyfin.ItemKindCollectionFolder, CollectionType: "tvshows"}},
		browse:    []jellyfin.Item{{ID: "series", Name: "Series", Type: jellyfin.ItemKindSeries}},
	}
	model := New(context.Background(), service)
	model, _ = update(t, model, model.Init()())
	model, cmd := update(t, model, key(tea.KeyEnter, ""))
	model, _ = update(t, model, cmd())
	if model.page.title != "Libraries" || len(model.stack) != 1 {
		t.Fatalf("libraries page/stack = %#v/%d", model.page, len(model.stack))
	}
	model, cmd = update(t, model, key(tea.KeyEnter, ""))
	model, _ = update(t, model, cmd())
	if service.parentID != "tv" || len(service.kinds) != 1 || service.kinds[0] != jellyfin.ItemKindSeries {
		t.Fatalf("browse = %q %#v", service.parentID, service.kinds)
	}
	model, _ = update(t, model, key(tea.KeyEscape, ""))
	if model.page.title != "Libraries" {
		t.Fatalf("back title = %q", model.page.title)
	}
}

func TestSearchTreatsQAsTextAndReturnsResults(t *testing.T) {
	service := &fakeService{search: []jellyfin.Item{{ID: "episode", Name: "Episode", Type: jellyfin.ItemKindEpisode}}}
	model := New(context.Background(), service)
	model, _ = update(t, model, model.Init()())
	model, _ = update(t, model, key('/', "/"))
	model, cmd := update(t, model, key('q', "q"))
	if cmd != nil || !model.searching || model.query != "q" {
		t.Fatalf("search state = %v %q", model.searching, model.query)
	}
	model, cmd = update(t, model, key(tea.KeyEnter, ""))
	model, _ = update(t, model, cmd())
	if service.searchTerm != "q" || model.page.title != "Search: q" || len(model.page.rows) != 1 {
		t.Fatalf("search = %q %#v", service.searchTerm, model.page)
	}
}

func TestPlayCommandDelegatesAndReturnsError(t *testing.T) {
	want := errors.New("player failed")
	service := &fakeService{playErr: want}
	err := (playCommand{ctx: context.Background(), service: service, itemID: "item"}).Run()
	if !errors.Is(err, want) || service.playedID != "item" {
		t.Fatalf("Run() = %v, played = %q", err, service.playedID)
	}
}

func TestRunRequiresService(t *testing.T) {
	if err := Run(context.Background(), nil); err == nil {
		t.Fatal("Run() error = nil")
	}
}
