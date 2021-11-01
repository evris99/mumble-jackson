package player

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
)

var ErrThumbDownload = errors.New("could not get thumbnail")

type Thumbnail struct {
	Data     []byte
	MimeType string
	URL      string
}

// Initializes a new thumbail from a give url string
func NewThumbnail(url string) (*Thumbnail, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrThumbDownload
	}

	buf := new(bytes.Buffer)
	writer := base64.NewEncoder(base64.StdEncoding, buf)
	_, err = io.Copy(writer, resp.Body)
	if err != nil {
		return nil, err
	}

	return &Thumbnail{
		URL:      url,
		MimeType: resp.Header.Get("content-type"),
		Data:     buf.Bytes(),
	}, nil
}
