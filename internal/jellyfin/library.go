package jellyfin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ItemKind is Jellyfin's BaseItemKind enum. Only kinds used by this client are
// named here; unknown future values still decode safely as strings.
type ItemKind string

const (
	ItemKindCollectionFolder ItemKind = "CollectionFolder"
	ItemKindMovie            ItemKind = "Movie"
	ItemKindSeries           ItemKind = "Series"
	ItemKindSeason           ItemKind = "Season"
	ItemKindEpisode          ItemKind = "Episode"
	ItemKindVideo            ItemKind = "Video"
)

type UserData struct {
	Played                bool     `json:"Played"`
	PlayedPercentage      *float64 `json:"PlayedPercentage"`
	PlaybackPositionTicks int64    `json:"PlaybackPositionTicks"`
	UnplayedItemCount     *int     `json:"UnplayedItemCount"`
}

// Item is the intentionally small subset of BaseItemDto required to browse and
// render playback state. Additional server fields are ignored by encoding/json.
type Item struct {
	ID                string    `json:"Id"`
	Name              string    `json:"Name"`
	Type              ItemKind  `json:"Type"`
	CollectionType    string    `json:"CollectionType"`
	MediaType         string    `json:"MediaType"`
	SeriesName        string    `json:"SeriesName"`
	ParentID          string    `json:"ParentId"`
	IndexNumber       *int      `json:"IndexNumber"`
	ParentIndexNumber *int      `json:"ParentIndexNumber"`
	RunTimeTicks      *int64    `json:"RunTimeTicks"`
	UserData          *UserData `json:"UserData"`
}

type ItemPage struct {
	Items            []Item `json:"Items"`
	TotalRecordCount int    `json:"TotalRecordCount"`
	StartIndex       int    `json:"StartIndex"`
}

type PageOptions struct {
	StartIndex int
	Limit      int
}

func (p PageOptions) add(values url.Values) error {
	if p.StartIndex < 0 || p.Limit < 0 {
		return errors.New("start index and limit must not be negative")
	}
	if p.StartIndex > 0 {
		values.Set("startIndex", strconv.Itoa(p.StartIndex))
	}
	if p.Limit > 0 {
		values.Set("limit", strconv.Itoa(p.Limit))
	}
	return nil
}

type ItemsQuery struct {
	Page         PageOptions
	ParentID     string
	IncludeTypes []ItemKind
	Recursive    bool
	SortBy       []string
	SortOrder    string
}

type ResumeQuery struct {
	Page         PageOptions
	ParentID     string
	IncludeTypes []ItemKind
}

type NextUpQuery struct {
	Page     PageOptions
	SeriesID string
	ParentID string
}

type LatestQuery struct {
	Limit        int
	ParentID     string
	IncludeTypes []ItemKind
}

// UserViews returns the libraries visible to a user.
func (c *Client) UserViews(ctx context.Context, userID string) (ItemPage, error) {
	values, err := c.userQuery(userID)
	if err != nil {
		return ItemPage{}, fmt.Errorf("get user views: %w", err)
	}
	values.Set("includeExternalContent", "false")
	var result ItemPage
	if err := c.doJSONQuery(ctx, http.MethodGet, "/UserViews", values, nil, &result); err != nil {
		return ItemPage{}, fmt.Errorf("get user views: %w", err)
	}
	return result, nil
}

// Items returns a page of movies, series, seasons, episodes, or other items.
func (c *Client) Items(ctx context.Context, userID string, query ItemsQuery) (ItemPage, error) {
	values, err := c.userQuery(userID)
	if err != nil {
		return ItemPage{}, fmt.Errorf("get items: %w", err)
	}
	if err := query.Page.add(values); err != nil {
		return ItemPage{}, fmt.Errorf("get items: %w", err)
	}
	setCommonItemQuery(values, query.ParentID, query.IncludeTypes)
	values.Set("recursive", strconv.FormatBool(query.Recursive))
	values.Set("enableUserData", "true")
	if len(query.SortBy) > 0 {
		values.Set("sortBy", strings.Join(query.SortBy, ","))
	}
	if query.SortOrder != "" {
		if query.SortOrder != "Ascending" && query.SortOrder != "Descending" {
			return ItemPage{}, errors.New("get items: sort order must be Ascending or Descending")
		}
		values.Set("sortOrder", query.SortOrder)
	}
	var result ItemPage
	if err := c.doJSONQuery(ctx, http.MethodGet, "/Items", values, nil, &result); err != nil {
		return ItemPage{}, fmt.Errorf("get items: %w", err)
	}
	return result, nil
}

func (c *Client) ResumeItems(ctx context.Context, userID string, query ResumeQuery) (ItemPage, error) {
	values, err := c.userQuery(userID)
	if err != nil {
		return ItemPage{}, fmt.Errorf("get resume items: %w", err)
	}
	if err := query.Page.add(values); err != nil {
		return ItemPage{}, fmt.Errorf("get resume items: %w", err)
	}
	setCommonItemQuery(values, query.ParentID, query.IncludeTypes)
	values.Set("mediaTypes", "Video")
	values.Set("enableUserData", "true")
	var result ItemPage
	if err := c.doJSONQuery(ctx, http.MethodGet, "/UserItems/Resume", values, nil, &result); err != nil {
		return ItemPage{}, fmt.Errorf("get resume items: %w", err)
	}
	return result, nil
}

func (c *Client) NextUp(ctx context.Context, userID string, query NextUpQuery) (ItemPage, error) {
	values, err := c.userQuery(userID)
	if err != nil {
		return ItemPage{}, fmt.Errorf("get next up: %w", err)
	}
	if err := query.Page.add(values); err != nil {
		return ItemPage{}, fmt.Errorf("get next up: %w", err)
	}
	if query.SeriesID != "" {
		values.Set("seriesId", query.SeriesID)
	}
	if query.ParentID != "" {
		values.Set("parentId", query.ParentID)
	}
	values.Set("enableUserData", "true")
	var result ItemPage
	if err := c.doJSONQuery(ctx, http.MethodGet, "/Shows/NextUp", values, nil, &result); err != nil {
		return ItemPage{}, fmt.Errorf("get next up: %w", err)
	}
	return result, nil
}

func (c *Client) Latest(ctx context.Context, userID string, query LatestQuery) ([]Item, error) {
	values, err := c.userQuery(userID)
	if err != nil {
		return nil, fmt.Errorf("get latest media: %w", err)
	}
	if query.Limit < 0 {
		return nil, errors.New("get latest media: limit must not be negative")
	}
	if query.Limit > 0 {
		values.Set("limit", strconv.Itoa(query.Limit))
	}
	setCommonItemQuery(values, query.ParentID, query.IncludeTypes)
	values.Set("enableUserData", "true")
	values.Set("groupItems", "false")
	var result []Item
	if err := c.doJSONQuery(ctx, http.MethodGet, "/Items/Latest", values, nil, &result); err != nil {
		return nil, fmt.Errorf("get latest media: %w", err)
	}
	return result, nil
}

func (c *Client) userQuery(userID string) (url.Values, error) {
	if c.token == "" {
		return nil, errors.New("access token is required")
	}
	if userID == "" {
		return nil, errors.New("user ID is required")
	}
	return url.Values{"userId": {userID}}, nil
}

func setCommonItemQuery(values url.Values, parentID string, includeTypes []ItemKind) {
	if parentID != "" {
		values.Set("parentId", parentID)
	}
	if len(includeTypes) > 0 {
		types := make([]string, len(includeTypes))
		for i, itemType := range includeTypes {
			types[i] = string(itemType)
		}
		values.Set("includeItemTypes", strings.Join(types, ","))
	}
}
