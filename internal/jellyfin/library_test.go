package jellyfin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestLibraryEndpointsAndDTOs(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantQuery url.Values
		response  string
		call      func(*Client) error
	}{
		{
			name: "views", path: "/UserViews",
			wantQuery: url.Values{"userId": {"user-id"}, "includeExternalContent": {"false"}},
			response:  `{"Items":[{"Id":"library-id","Name":"Movies","Type":"CollectionFolder","CollectionType":"movies"}],"TotalRecordCount":1,"StartIndex":0}`,
			call: func(c *Client) error {
				page, err := c.UserViews(context.Background(), "user-id")
				if err == nil && (len(page.Items) != 1 || page.Items[0].CollectionType != "movies") {
					t.Errorf("UserViews() = %#v", page)
				}
				return err
			},
		},
		{
			name: "items", path: "/Items",
			wantQuery: url.Values{
				"userId": {"user-id"}, "parentId": {"series-id"}, "includeItemTypes": {"Season,Episode"},
				"recursive": {"true"}, "enableUserData": {"true"}, "startIndex": {"10"}, "limit": {"20"},
				"sortBy": {"SortName"}, "sortOrder": {"Ascending"},
			},
			response: `{"Items":[{"Id":"episode-id","Name":"Pilot","Type":"Episode","SeriesName":"Example","IndexNumber":1,"ParentIndexNumber":2,"RunTimeTicks":36000000000,"UserData":{"Played":false,"PlayedPercentage":25.5,"PlaybackPositionTicks":9000000000}}],"TotalRecordCount":1,"StartIndex":10}`,
			call: func(c *Client) error {
				page, err := c.Items(context.Background(), "user-id", ItemsQuery{
					Page: PageOptions{StartIndex: 10, Limit: 20}, ParentID: "series-id",
					IncludeTypes: []ItemKind{ItemKindSeason, ItemKindEpisode}, Recursive: true,
					SortBy: []string{"SortName"}, SortOrder: "Ascending",
				})
				if err == nil && (len(page.Items) != 1 || page.Items[0].IndexNumber == nil || *page.Items[0].IndexNumber != 1 || page.Items[0].UserData == nil || page.Items[0].UserData.PlaybackPositionTicks != 9000000000) {
					t.Errorf("Items() = %#v", page)
				}
				return err
			},
		},
		{
			name: "search and IDs", path: "/Items",
			wantQuery: url.Values{
				"userId": {"user-id"}, "searchTerm": {"pilot"}, "ids": {"one,two"},
				"recursive": {"true"}, "enableUserData": {"true"},
			},
			response: `{"Items":[],"TotalRecordCount":0,"StartIndex":0}`,
			call: func(c *Client) error {
				_, err := c.Items(context.Background(), "user-id", ItemsQuery{
					SearchTerm: "pilot", ItemIDs: []string{"one", "two"}, Recursive: true,
				})
				return err
			},
		},
		{
			name: "resume", path: "/UserItems/Resume",
			wantQuery: url.Values{"userId": {"user-id"}, "limit": {"12"}, "includeItemTypes": {"Movie,Episode"}, "mediaTypes": {"Video"}, "enableUserData": {"true"}},
			response:  `{"Items":[],"TotalRecordCount":0,"StartIndex":0}`,
			call: func(c *Client) error {
				_, err := c.ResumeItems(context.Background(), "user-id", ResumeQuery{Page: PageOptions{Limit: 12}, IncludeTypes: []ItemKind{ItemKindMovie, ItemKindEpisode}})
				return err
			},
		},
		{
			name: "next up", path: "/Shows/NextUp",
			wantQuery: url.Values{"userId": {"user-id"}, "limit": {"8"}, "seriesId": {"series-id"}, "enableUserData": {"true"}},
			response:  `{"Items":[],"TotalRecordCount":0,"StartIndex":0}`,
			call: func(c *Client) error {
				_, err := c.NextUp(context.Background(), "user-id", NextUpQuery{Page: PageOptions{Limit: 8}, SeriesID: "series-id"})
				return err
			},
		},
		{
			name: "latest", path: "/Items/Latest",
			wantQuery: url.Values{"userId": {"user-id"}, "parentId": {"library-id"}, "includeItemTypes": {"Movie"}, "limit": {"16"}, "enableUserData": {"true"}, "groupItems": {"false"}},
			response:  `[{"Id":"movie-id","Name":"Movie","Type":"Movie"}]`,
			call: func(c *Client) error {
				items, err := c.Latest(context.Background(), "user-id", LatestQuery{Limit: 16, ParentID: "library-id", IncludeTypes: []ItemKind{ItemKindMovie}})
				if err == nil && (len(items) != 1 || items[0].Type != ItemKindMovie) {
					t.Errorf("Latest() = %#v", items)
				}
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != tt.path {
					t.Errorf("request = %s %s, want GET %s", r.Method, r.URL.Path, tt.path)
				}
				if got := r.URL.Query(); !queryEqual(got, tt.wantQuery) {
					t.Errorf("query = %#v, want %#v", got, tt.wantQuery)
				}
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()
			if err := tt.call(newTestClient(t, server.URL).WithToken("token")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLibraryQueriesValidateBeforeRequest(t *testing.T) {
	client := newTestClient(t, "https://media.example.test").WithToken("token")
	if _, err := client.Items(context.Background(), "", ItemsQuery{}); err == nil {
		t.Fatal("Items() missing user error = nil")
	}
	if _, err := client.Items(context.Background(), "user", ItemsQuery{Page: PageOptions{Limit: -1}}); err == nil {
		t.Fatal("Items() negative limit error = nil")
	}
	if _, err := client.Latest(context.Background(), "user", LatestQuery{Limit: -1}); err == nil {
		t.Fatal("Latest() negative limit error = nil")
	}
	if _, err := newTestClient(t, "https://media.example.test").UserViews(context.Background(), "user"); err == nil {
		t.Fatal("UserViews() missing token error = nil")
	}
}

func queryEqual(a, b url.Values) bool {
	return a.Encode() == b.Encode()
}
