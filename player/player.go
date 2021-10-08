package player

import (
	"errors"
	"log"
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
	ErrNoFormat    = errors.New("no format found")
	ErrEmpty       = errors.New("empty playlist")
	ErrPlaying     = errors.New("the playlist is already playing")
	ErrStopped     = errors.New("the playlist is already stopped")
	ErrVolumeRange = errors.New("the volume level is incorrect")
)

type Player struct {
	streams       chan *gumbleffmpeg.Stream
	currentStream *gumbleffmpeg.Stream
	playing       bool
	skip          chan bool
	stop          chan bool
	volume        float32
	streamMutex   *sync.Mutex
}

// Creates and returns a Player instance
func New() *Player {
	return &Player{
		streams:     make(chan *gumbleffmpeg.Stream, MaxPlaylistSize),
		playing:     false,
		skip:        make(chan bool),
		stop:        make(chan bool),
		volume:      0.6,
		streamMutex: new(sync.Mutex),
	}
}

// Add the song from the URL to the playlist
func (p *Player) AddToQueue(c *gumble.Client, url *url.URL) error {
	stream, err := getStream(url, c)
	if err != nil {
		return err
	}

	p.streams <- stream
	return nil
}

func (p *Player) ClearQueue() {
	for {
		select {
		case <-p.streams:
		default:
			return
		}
	}
}

// Receives an integer between 0-100 and sets the volume to that value.
// Returns an error if the number is not in the correct range
func (p *Player) SetVolume(vol int) error {
	if vol < 0 || vol > 100 {
		return ErrVolumeRange
	}

	p.volume = float32(vol) / 100

	p.streamMutex.Lock()
	if p.currentStream != nil {
		p.currentStream.Volume = p.volume
	}
	p.streamMutex.Unlock()

	return nil
}

// Skips a song from the playlist
func (p *Player) Skip() error {
	if len(p.streams) == 0 {
		return ErrEmpty
	}

	if !p.playing {
		<-p.streams
		return nil
	}

	p.skip <- true
	return nil
}

// Starts the playlist
func (p *Player) Start(c *gumble.Client) error {
	if p.playing {
		return ErrPlaying
	}

	if len(p.streams) == 0 {
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

func (p *Player) SearchAndAdd(c *gumble.Client, apiKey, query string) (string, error) {
	rawURL, err := youtube_search.Search(query, apiKey)
	if err != nil {
		return "", err
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	p.AddToQueue(c, parsedURL)

	return rawURL, nil
}

// Start playing song from the playlist
// until it receives from the stop channel
func (p *Player) startPlaylist(c *gumble.Client) {
	stop := false

	for p.currentStream = range p.streams {
		finished := make(chan bool)

		p.streamMutex.Lock()
		p.currentStream.Volume = p.volume
		p.streamMutex.Unlock()

		go playStream(p.currentStream, finished)

		select {
		case <-p.stop:
			p.streamMutex.Lock()
			p.currentStream.Stop()
			p.streamMutex.Unlock()
			stop = true
		case <-p.skip:
			p.streamMutex.Lock()
			p.currentStream.Stop()
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

// Receives a URL and returns an audio stream or an error
func getStream(u *url.URL, client *gumble.Client) (*gumbleffmpeg.Stream, error) {
	var streamURL string
	var err error

	switch u.Host {
	case "www.youtube.com":
		streamURL, err = getYoutubeStreamURL(u)
	}

	if err != nil {
		return nil, err
	}

	source := gumbleffmpeg.SourceFile(streamURL)
	return gumbleffmpeg.New(client, source), nil
}

// Receives a youtube video URL and returns
// the streaming URL or an error
func getYoutubeStreamURL(u *url.URL) (string, error) {
	client := &youtube.Client{}
	video, err := client.GetVideo(u.String())
	if err != nil {
		return "", err
	}
	form, err := findBestFormat(video.Formats)
	if err != nil {
		return "", err
	}

	return client.GetStreamURL(video, form)
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
