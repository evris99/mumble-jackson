package player

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/evris99/mumble-music-bot/youtube_search"
	"github.com/kkdai/youtube/v2"
	"layeh.com/gumble/gumble"
	"layeh.com/gumble/gumbleffmpeg"
	_ "layeh.com/gumble/opus"
)

const MaxPlaylistSize = 100
const MaxNextSongs = 20

var (
	ErrNoFormat      = errors.New("no format found")
	ErrEmpty         = errors.New("empty playlist")
	ErrPlaying       = errors.New("the playlist is already playing")
	ErrStopped       = errors.New("the playlist is already stopped")
	ErrVolumeRange   = errors.New("the volume level is incorrect")
	ErrEmptyPlaylist = errors.New("playlist empty")
	ErrIncorrectURL  = errors.New("incorrect url")
)

type Player struct {
	tracks       chan *Track
	currentTrack *Track
	SongList     []string
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
		SongList:    make([]string, 0),
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

	p.SongList = p.SongList[1:]
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

	p.SongList = p.SongList[1:]
	p.skip <- true
	return nil
}

// Add the song from the URL to the playlist
// Returns the track that is added.
func (p *Player) AddToQueue(c *gumble.Client, url *url.URL) ([]*Track, error) {
	redirectURL, err := getRedirectURL(url)
	if err != nil {
		return nil, err
	}

	tracks, err := getURLTracks(redirectURL, c)
	if err != nil {
		return nil, err
	}

	for _, track := range tracks {
		p.SongList = append(p.SongList, track.Title)
		p.tracks <- track
	}

	return tracks, nil
}

// Get songs that are going to play next
func (p *Player) GetNextSongs() (string, error) {
	if len(p.tracks) == 0 {
		return "", ErrEmpty
	}
	songlist := "<br><b>"
	for i, songname := range p.SongList {
		if i == MaxNextSongs {
			songlist += ". . . . <br>"
			break
		}
		songlist += strconv.Itoa(i+1) + ": " + songname + "<br>"
	}
	songlist += "</b>"
	return songlist, nil
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

	tracks, err := p.AddToQueue(c, parsedURL)
	if err != nil {
		return nil, err
	}

	if len(tracks) != 1 {
		return nil, ErrIncorrectURL
	}

	return tracks[0], nil
}

// Clears the tracks from the playlist
func (p *Player) ClearQueue() {
	p.SongList = nil
	for {
		select {
		case <-p.tracks:
		default:
			return
		}
	}
}

// Returns info about the current song
func (p *Player) GetCurrentSong() (string, error) {
	if p.currentTrack == nil || p.currentTrack.Stream == nil {
		return "", ErrEmpty
	}
	currentTime := formatDuration(p.currentTrack.Stream.Elapsed())
	totalTime := formatDuration(p.currentTrack.Duration)
	progress := getProgressBar(p.currentTrack.Duration, p.currentTrack.Stream.Elapsed())
	return fmt.Sprintf("<h4>%s ▶ %s %s</h4>%v", currentTime, progress, totalTime, p.currentTrack), nil
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

		p.currentTrack = nil

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
func getURLTracks(u *url.URL, client *gumble.Client) ([]*Track, error) {
	switch u.Host {
	case "www.youtube.com":
		return getYoutubeTracks(client, u)
	default:
		return nil, ErrIncorrectURL
	}
}

// Receives a youtube video URL and returns
// the streaming URL or an error
func getYoutubeTracks(c *gumble.Client, u *url.URL) ([]*Track, error) {
	client := new(youtube.Client)
	switch u.Path {
	case "/watch":
		video, err := client.GetVideo(u.String())
		if err != nil {
			return nil, err
		}

		track, err := YoutubeVideoToTrack(c, client, video)
		return []*Track{track}, err
	case "/playlist":
		playlist, err := client.GetPlaylist(u.String())
		if err != nil {
			return nil, err
		}

		return YoutubePlaylistToTracks(c, client, playlist)
	default:
		return nil, ErrIncorrectURL
	}
}

// Receives a url and returns the final redirection url
func getRedirectURL(url *url.URL) (*url.URL, error) {
	resp, err := http.Head(url.String())
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, ErrIncorrectURL
	}

	return resp.Request.URL, nil
}
