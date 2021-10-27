package player

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/evris99/mumble-music-bot/youtube_search"
	"github.com/kkdai/youtube/v2"
	"layeh.com/gumble/gumble"
	"layeh.com/gumble/gumbleffmpeg"
	_ "layeh.com/gumble/opus"
)

const MaxPlaylistSize = 100

var (
	ErrNoFormat      = errors.New("no format found")
	ErrEmpty         = errors.New("empty playlist")
	ErrPlaying       = errors.New("the playlist is already playing")
	ErrStopped       = errors.New("the playlist is already stopped")
	ErrVolumeRange   = errors.New("the volume level is incorrect")
	ErrThumbDownload = errors.New("could not get thumbnail")
	ErrThumbNoURL    = errors.New("no URL found for thumbnail")
)

type VideoDuration struct {
	Hours   int
	Minutes int
	Seconds int
}

type Thumbnail struct {
	Data     []byte
	MimeType string
	URL      string
}

// Downloads the thumbnail and adds it to the data.
// If the URL field is not set it throws an error
func (t *Thumbnail) GetThumbnail() error {
	if t.URL == "" {
		return ErrThumbNoURL
	}

	resp, err := http.Get(t.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return ErrThumbDownload
	}
	t.MimeType = resp.Header.Get("content-type")

	buf := new(bytes.Buffer)
	writer := base64.NewEncoder(base64.StdEncoding, buf)
	_, err = io.Copy(writer, resp.Body)
	if err != nil {
		return err
	}

	t.Data = buf.Bytes()
	return nil
}

type Track struct {
	Stream    *gumbleffmpeg.Stream
	Duration  *VideoDuration
	StreamURL string
	PublicURL string
	Title     string
	Artist    string
	Thumbnail *Thumbnail
}

// Returns the string for displaying the track
func (t *Track) GetMessage() string {
	title := fmt.Sprintf("<h3 style=\"margin: 0px; padding: 0px;\"><a style=\"margin: 0px; padding: 0px;\" href=\"%s\">%s</a></h3>", t.PublicURL, t.Title)
	artist := fmt.Sprintf("<h4 style=\"margin: 0px; padding: 0px;\"> by %s</h4>", t.Artist)
	duration := fmt.Sprintf(" %v Hours %v Minutes %v Seconds <br>", t.Duration.Hours, t.Duration.Minutes, t.Duration.Seconds)
	image := fmt.Sprintf("<img style=\"float: left; padding:0px;\"src=\"data:%s;base64,%s\"/><br>", t.Thumbnail.MimeType, string(t.Thumbnail.Data))
	return fmt.Sprintf("%s%s%s%s", title, artist, duration, image)
}

type Player struct {
	tracks       chan *Track
	CurrentTrack *Track
	playing      bool
	skip         chan bool
	stop         chan bool
	volume       float32
	streamMutex  *sync.Mutex
}

// Creates and returns a Player instance
func New(default_volume uint8) *Player {
	return &Player{
		tracks:      make(chan *Track, MaxPlaylistSize),
		playing:     false,
		skip:        make(chan bool),
		stop:        make(chan bool),
		volume:      float32(default_volume) / 100,
		streamMutex: new(sync.Mutex),
	}
}

// Starts the playlist
func (p *Player) Start(c *gumble.Client) error {
	if p.playing {
		return ErrPlaying
	}

	if len(p.tracks) == 0 {
		return ErrEmpty
	}

	p.playing = true
	go p.startPlaylist(c)
	return nil
}

// Stops the playlist
func (p *Player) Stop() error {
	if !p.playing {
		return ErrStopped
	}

	p.playing = false
	p.stop <- true
	return nil
}

// Skips a song from the playlist
func (p *Player) Skip() error {
	if len(p.tracks) == 0 {
		if !p.playing {
			return ErrEmpty
		} else {
			return p.Stop()
		}

	}

	if !p.playing {
		<-p.tracks
		return nil
	}
	p.skip <- true
	return nil
}

// Add the song from the URL to the playlist
// Returns the track that is added.
func (p *Player) AddToQueue(c *gumble.Client, url *url.URL) (*Track, error) {
	track, err := getURLTrack(url, c)
	if err != nil {
		return nil, err
	}

	if err = track.Thumbnail.GetThumbnail(); err != nil {
		return nil, err
	}

	p.tracks <- track
	return track, nil
}

// Searches youtube using the query argument and adds the first result to the playlist.
// Returns the track that is added.
func (p *Player) SearchAndAdd(c *gumble.Client, apiKey, query string) (*Track, error) {
	rawURL, err := youtube_search.Search(query, apiKey)
	if err != nil {
		return nil, err
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	return p.AddToQueue(c, parsedURL)
}

// Clears the tracks from the playlist
func (p *Player) ClearQueue() {
	for {
		select {
		case <-p.tracks:
		default:
			return
		}
	}
}

// Returns the current volume in float (Range: 0 - 1)
func (p *Player) GetVolume() float32 {
	return p.volume
}

// Receives an integer between 0-100 and sets the volume to that value.
// Returns an error if the number is not in the correct range
func (p *Player) SetVolume(vol int) error {
	if vol < 0 || vol > 100 {
		return ErrVolumeRange
	}

	p.volume = float32(vol) / 100

	p.streamMutex.Lock()
	if p.CurrentTrack != nil && p.CurrentTrack.Stream != nil {
		p.CurrentTrack.Stream.Volume = p.volume
	}
	p.streamMutex.Unlock()

	return nil
}

// Start playing song from the playlist
// until it receives from the stop channel
func (p *Player) startPlaylist(c *gumble.Client) {
	stop := false

	for p.CurrentTrack = range p.tracks {
		finished := make(chan bool)

		p.streamMutex.Lock()
		p.CurrentTrack.Stream.Volume = p.volume
		p.streamMutex.Unlock()

		go playStream(p.CurrentTrack.Stream, finished)

		select {
		case <-p.stop:
			p.streamMutex.Lock()
			p.CurrentTrack.Stream.Stop()
			p.streamMutex.Unlock()
			stop = true
		case <-p.skip:
			p.streamMutex.Lock()
			p.CurrentTrack.Stream.Stop()
			p.streamMutex.Unlock()
		case <-finished:
		}

		if stop {
			break
		}
	}
}

// Receives an audio stream and a channel. It plays the stream
// and send a message to the channel when it is finished.
func playStream(s *gumbleffmpeg.Stream, finished chan bool) {
	err := s.Play()
	if err != nil {
		log.Fatalln(err)
	}

	s.Wait()
	finished <- true
}

// Returns the given player track channel(s)
func (p *Player) GetCurrentStream() (*gumbleffmpeg.Stream, error) {
	if !p.playing {
		return nil, ErrEmpty
	}
	s := p.CurrentTrack.Stream
	return s, nil
}

// Receives a URL and returns an audio stream or an error
func getURLTrack(u *url.URL, client *gumble.Client) (*Track, error) {
	var track *Track
	var err error
	switch u.Host {
	case "www.youtube.com", "youtu.be":
		track, err = getYoutubeTrack(u)
	}

	if err != nil {
		return nil, err
	}

	source := gumbleffmpeg.SourceFile(track.StreamURL)
	track.Stream = gumbleffmpeg.New(client, source)
	return track, nil
}

// Receives a youtube video URL and returns
// the streaming URL or an error
func getYoutubeTrack(u *url.URL) (*Track, error) {
	client := &youtube.Client{}
	video, err := client.GetVideo(u.String())
	if err != nil {
		return nil, err
	}

	form, err := findBestFormat(video.Formats)
	if err != nil {
		return nil, err
	}

	url, err := client.GetStreamURL(video, form)
	if err != nil {
		return nil, err
	}
	duration := VideoDuration{int(video.Duration.Hours()), int(video.Duration.Minutes()), int(video.Duration.Seconds())}
	track := &Track{
		Title:     video.Title,
		Artist:    video.Author,
		Duration:  &duration,
		StreamURL: url,
		PublicURL: u.String(),
		Thumbnail: &Thumbnail{URL: ""},
	}

	if len(video.Thumbnails) > 0 {
		track.Thumbnail.URL = video.Thumbnails[0].URL
	}

	return track, err
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
