package youtube_search

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

var (
	ErrRequest       error = errors.New("could not fetch data from youtube")
	ErrEmptyResponse error = errors.New("the search result is empty")
)

type ID struct {
	VideoID string `json:"videoId"`
}

type Item struct {
	ItemID ID `json:"id"`
}

type SearchResponse struct {
	Items []Item `json:"items"`
}

// Returns the URL of the first search result from Youtube based on the query
func Search(query, apiKey string) (string, error) {
	url, err := getApiURL(query, apiKey)
	if err != nil {
		return "", err
	}

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", ErrRequest
	}

	id, err := extractID(resp.Body)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("https://www.youtube.com/watch?v=%s", id), nil
}

// Returns the url for making the search request based on the query and the API key
func getApiURL(query, apiKey string) (string, error) {
	queryParams := make(url.Values, 4)
	queryParams.Add("part", "id")
	queryParams.Add("q", query)
	queryParams.Add("key", apiKey)
	queryParams.Add("type", "video")

	u, err := url.Parse("https://www.googleapis.com/youtube/v3/search")
	if err != nil {
		return "", err
	}
	u.RawQuery = queryParams.Encode()

	return u.String(), nil
}

// Extracts and returns the video ID from the http response
func extractID(body io.ReadCloser) (string, error) {
	decoder := json.NewDecoder(body)
	searchRes := new(SearchResponse)
	err := decoder.Decode(searchRes)
	if err != nil {
		return "", err
	}

	if len(searchRes.Items) == 0 {
		return "", ErrEmptyResponse
	}

	return searchRes.Items[0].ItemID.VideoID, nil
}
