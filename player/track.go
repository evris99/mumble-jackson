package player

import (
	"fmt"
	"time"

	"github.com/kkdai/youtube/v2"
	"layeh.com/gumble/gumble"
	"layeh.com/gumble/gumbleffmpeg"
)

type Track struct {
	Stream    *gumbleffmpeg.Stream
	Duration  time.Duration
	StreamURL string
	PublicURL string
	Title     string
	Artist    string
	Thumbnail *Thumbnail
}

// Returns the string for displaying the track
func (t *Track) String() string {
	title := fmt.Sprintf("<h3 style=\"margin: 0px; padding: 0px;\"><a style=\"margin: 0px; padding: 0px;\" href=\"%s\">%s</a></h3>", t.PublicURL, t.Title)
	artist := fmt.Sprintf("<h4 style=\"margin: 0px; padding: 0px;\"> by %s</h4>", t.Artist)
	duration := fmt.Sprintf("%s<br>", formatDuration(t.Duration))
	image := fmt.Sprintf("<img style=\"float: left; padding:0px;\"src=\"data:%s;base64,%s\"/><br>", t.Thumbnail.MimeType, string(t.Thumbnail.Data))
	return fmt.Sprintf("%s%s%s%s", title, artist, duration, image)
}

// Receives a youtube video and returns a track struct
func YoutubeVideoToTrack(gc *gumble.Client, yc *youtube.Client, video *youtube.Video) (*Track, error) {
	form, err := findBestFormat(video.Formats)
	if err != nil {
		return nil, err
	}

	url, err := yc.GetStreamURL(video, form)
	if err != nil {
		return nil, err
	}

	var thumbnail *Thumbnail
	if len(video.Thumbnails) > 0 {
		thumbnail, err = NewThumbnail(video.Thumbnails[0].URL)
		if err != nil {
			return nil, err
		}
	}

	return &Track{
		Title:     video.Title,
		Artist:    video.Author,
		Duration:  video.Duration,
		StreamURL: url,
		PublicURL: fmt.Sprintf("https://www.youtube.com/watch?v=%s", video.ID),
		Thumbnail: thumbnail,
		Stream:    gumbleffmpeg.New(gc, gumbleffmpeg.SourceFile(url)),
	}, nil
}

// Returns a slice of tracks from a playlist
func YoutubePlaylistToTracks(gc *gumble.Client, yc *youtube.Client, p *youtube.Playlist) ([]*Track, error) {
	if len(p.Videos) == 0 {
		return nil, ErrEmptyPlaylist
	}

	tracks := make([]*Track, 0)
	errChan := make(chan error)
	trackChan := make(chan *Track)

	// Get track concurrently
	for _, entry := range p.Videos {
		go func(e *youtube.PlaylistEntry) {
			video, err := yc.VideoFromPlaylistEntry(e)
			if err != nil {
				errChan <- err
				return
			}

			track, err := YoutubeVideoToTrack(gc, yc, video)
			if err != nil {
				errChan <- err
				return
			}

			trackChan <- track
		}(entry)

	}

	counter := len(p.Videos)
	for counter > 0 {
		select {
		case track := <-trackChan:
			tracks = append(tracks, track)
			counter--
		case err := <-errChan:
			return nil, err
		}
	}

	return tracks, nil
}

// Finds the best audio formats for a format list
// and returns an error if no format is found
func findBestFormat(formats youtube.FormatList) (*youtube.Format, error) {
	f := formats.FindByItag(251)
	if f != nil {
		return f, nil
	}

	f = formats.FindByItag(250)
	if f != nil {
		return f, nil
	}

	f = formats.FindByItag(249)
	if f != nil {
		return f, nil
	}

	return nil, ErrNoFormat
}
