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
	"time"

	"github.com/evris99/mumble-music-bot/youtube_search"
	"github.com/kkdai/youtube/v2"
	"golang.org/x/sync/errgroup"
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
	Duration  time.Duration
	StreamURL string
	PublicURL string
	Title     string
	Artist    string
	Thumbnail *Thumbnail
}

// Receives a youtube video and returns a track struct
func getTrackFromVideo(client *youtube.Client, video *youtube.Video) (*Track, error) {
	form, err := findBestFormat(video.Formats)
	if err != nil {
		return nil, err
	}

	url, err := client.GetStreamURL(video, form)
	if err != nil {
		return nil, err
	}

	track := &Track{
		Title:     video.Title,
		Artist:    video.Author,
		Duration:  video.Duration,
		StreamURL: url,
		PublicURL: fmt.Sprintf("https://www.youtube.com/watch?v=%s", video.ID),
		Thumbnail: &Thumbnail{URL: ""},
	}

	if len(video.Thumbnails) > 0 {
		track.Thumbnail.URL = video.Thumbnails[0].URL
	}

	return track, nil
}

// Returns the string for displaying the track
func (t *Track) GetMessage() string {
	url := fmt.Sprintf("<a href=\"%[1]s\">%[1]s</a><br>", t.PublicURL)
	title := fmt.Sprintf("<h3 style=\"margin: 0px; padding: 0px;\"><a style=\"margin: 0px; padding: 0px;\" href=\"%s\">%s</a></h3>", t.PublicURL, t.Title)
	artist := fmt.Sprintf("<h4 style=\"margin: 0px; padding: 0px;\"> by %s</h4>", t.Artist)
	duration := fmt.Sprintf("%s<br>", formatDuration(t.Duration))
	image := fmt.Sprintf("<img style=\"float: left; padding:0px;\"src=\"data:%s;base64,%s\"/><br>", t.Thumbnail.MimeType, string(t.Thumbnail.Data))
	return fmt.Sprintf("%s%s%s%s%s", url, title, artist, duration, image)
}

type Player struct {
	tracks       chan *Track
	currentTrack *Track
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

// Receives a youtube plalist URL, adds it to the queue and returns the number of tracks in it
func (p *Player) AddPlaylist(c *gumble.Client, url *url.URL) (int, error) {
	tracks, err := getPlaylistTracks(url, c)
	if err != nil {
		return 0, err
	}

	// Get thumbnails in parallel
	wg := new(errgroup.Group)
	for _, track := range tracks {
		wg.Go(getThumbnailCallback(track))
	}

	if err := wg.Wait(); err != nil {
		return 0, err
	}

	for _, track := range tracks {
		p.tracks <- track
	}

	return len(tracks), nil
}

// Returns a callback function for getting a track's thumbnail
func getThumbnailCallback(t *Track) func() error {
	return func() error {
		return t.Thumbnail.GetThumbnail()
	}
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

// Returns info about the current song
func (p *Player) GetCurrentSong() string {
	currentTime := formatDuration(p.currentTrack.Stream.Elapsed())
	totalTime := formatDuration(p.currentTrack.Duration)
	progress := getProgressBar(p.currentTrack.Duration, p.currentTrack.Stream.Elapsed())
	header := p.currentTrack.GetMessage()
	return fmt.Sprintf("<h4>%s ▶ %s %s</h4>%s", currentTime, progress, totalTime, header)
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
	if p.currentTrack != nil && p.currentTrack.Stream != nil {
		p.currentTrack.Stream.Volume = p.volume
	}
	p.streamMutex.Unlock()

	return nil
}

// Start playing song from the playlist
// until it receives from the stop channel
func (p *Player) startPlaylist(c *gumble.Client) {
	stop := false

	for p.currentTrack = range p.tracks {
		finished := make(chan bool)

		p.streamMutex.Lock()
		p.currentTrack.Stream.Volume = p.volume
		p.streamMutex.Unlock()
		go playStream(p.currentTrack.Stream, finished)
		select {
		case <-p.stop:
			p.streamMutex.Lock()
			p.currentTrack.Stream.Stop()
			p.streamMutex.Unlock()
			stop = true
		case <-p.skip:
			p.streamMutex.Lock()
			p.currentTrack.Stream.Stop()
			p.streamMutex.Unlock()
		case <-finished:
			if len(p.tracks) == 0 {
				p.playing = false
				stop = true
			}
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

// Creates and returns a string with the format "hh:mm:ss"
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// Creates and returns a progress bar with unicode characters
func getProgressBar(total, elapsed time.Duration) string {
	//returns value 0 - 1 (0 = just started, 1 = finished)
	percentage_played := 1 - (total.Seconds()-elapsed.Seconds())/total.Seconds()
	track_lines := ""
	track_current_place := int(percentage_played * 10)
	for i := 0; i < 10; i++ {
		if i == track_current_place {
			track_lines += "🔶"
		} else if i < track_current_place {
			track_lines += "🟦"
		} else {
			track_lines += "➖"
		}
	}
	return track_lines
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
	client := new(youtube.Client)
	video, err := client.GetVideo(u.String())
	if err != nil {
		return nil, err
	}

	return getTrackFromVideo(client, video)
}

// Receives a youtube playlist URL and returns a slice of tracks or an error
func getPlaylistTracks(u *url.URL, c *gumble.Client) ([]*Track, error) {
	client := new(youtube.Client)
	playlist, err := client.GetPlaylist(u.String())
	if err != nil {
		return nil, err
	}

	// Get tracks in parallel
	counter := len(playlist.Videos)
	trackChan := make(chan *Track)
	errChan := make(chan error)
	for _, entry := range playlist.Videos {
		go getTrackStream(c, client, entry, trackChan, errChan)
	}

	tracks := make([]*Track, 0)
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

// Gets Tracks and send them through the trackChan channel
func getTrackStream(mc *gumble.Client, yc *youtube.Client, entry *youtube.PlaylistEntry, trackChan chan<- *Track, errChan chan<- error) {
	video, err := yc.VideoFromPlaylistEntry(entry)
	if err != nil {
		errChan <- err
		return
	}

	track, err := getTrackFromVideo(yc, video)
	if err != nil {
		errChan <- err
		return
	}

	source := gumbleffmpeg.SourceFile(track.StreamURL)
	track.Stream = gumbleffmpeg.New(mc, source)
	trackChan <- track
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
