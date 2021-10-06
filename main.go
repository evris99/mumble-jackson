package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/evris99/mumble-music-bot/player"
	"layeh.com/gumble/gumble"
	"layeh.com/gumble/gumbleutil"
	_ "layeh.com/gumble/opus"
	"mvdan.cc/xurls/v2"
)

var (
	ErrCertFile   = errors.New("cert file missing")
	ErrKeyFile    = errors.New("key file missing")
	ErrTooFewArgs = errors.New("too few arguments in command")
	ErrNoURLFound = errors.New("no url source found")
)

const helpmessage string = `<h2>Usage</h2><br>
start: Starts the playlist.<br>
stop: Stops the playlist.<br>
add $url: Add the youtube URL to playlist.<br>
skip: Skips a track from the playlist.<br>
vol $num: Sets the volume to the specified number. The number must be between 0-100.<br>
help: Shows this message.<br>`

// The configuration for the TLS certificates
type CertConfig struct {
	UseCertificate bool   `toml:"use_certificate"`
	CertFile       string `toml:"certificate_file_path"`
	KeyFile        string `toml:"key_file_path"`
}

// The global configuration
type Config struct {
	Address           string      `toml:"address"`
	Port              uint16      `toml:"port"`
	Username          string      `toml:"username"`
	Prefix            string      `toml:"command_prefix"`
	VerifyCertificate bool        `toml:"verify_server_certificate"`
	CertConf          *CertConfig `toml:"certificate"`
}

func main() {
	// Get config file path from cli argument
	confPath := flag.String("c", "./configuration.toml", "the path to the configuration flag")
	flag.Parse()
	config := loadConfig(*confPath)

	gumbleConf := gumble.NewConfig()
	gumbleConf.Username = config.Username

	player := player.New()
	gumbleConf.Attach(gumbleutil.Listener{
		TextMessage: handleMessage(player, config.Prefix),
	})

	tlsConf, tlsErr := getTLSConfig(*config)
	if tlsErr != nil {
		log.Fatalln(tlsErr)
	}

	address := fmt.Sprintf("%s:%d", config.Address, config.Port)
	_, err := gumble.DialWithDialer(new(net.Dialer), address, gumbleConf, tlsConf)
	if err != nil {
		log.Fatalln(err)
	}

	// Block forever
	select {}
}

// Loads the config from the path argument and returns the config
func loadConfig(path string) *Config {
	conf := &Config{
		Username:          "music_bot",
		Address:           "localhost",
		Prefix:            "!",
		Port:              64738,
		VerifyCertificate: false,
		CertConf:          new(CertConfig),
	}

	_, err := toml.DecodeFile(path, conf)
	if err != nil {
		log.Fatalln(err)
	}

	return conf
}

// Receives the program's config and returns
// the corresponding TLS config
func getTLSConfig(c Config) (*tls.Config, error) {
	resConf := &tls.Config{InsecureSkipVerify: !c.VerifyCertificate}
	if !c.CertConf.UseCertificate {
		return resConf, nil
	}

	if c.CertConf.CertFile == "" {
		return nil, ErrCertFile
	}

	if c.CertConf.KeyFile == "" {
		return nil, ErrKeyFile
	}

	cert, err := tls.LoadX509KeyPair(c.CertConf.CertFile, c.CertConf.KeyFile)
	if err != nil {
		return nil, err
	}

	resConf.Certificates = []tls.Certificate{cert}
	return resConf, nil
}

// Returns a function to handle the text message event
func handleMessage(player *player.Player, prefix string) func(e *gumble.TextMessageEvent) {
	return func(e *gumble.TextMessageEvent) {
		if !strings.HasPrefix(e.Message, prefix) {
			return
		}

		var response string
		var err error
		words := strings.Fields(strings.TrimPrefix(e.Message, prefix))

		switch words[0] {
		case "start":
			response, err = onStart(player, e.Client)
		case "add":
			response, err = onAdd(player, e.Client, words)
		case "stop":
			response, err = onStop(player)
		case "skip":
			response, err = onSkip(player)
		case "vol":
			response, err = onVolume(player, words)
		case "help":
			response, err = helpmessage, nil
		}

		if handleError(err, e.Client) {
			e.Client.Self.Channel.Send(response, false)
		}
	}
}

// Starts the playlist and returns the corresponding answer or an error
func onStart(p *player.Player, c *gumble.Client) (string, error) {
	response := "Playlist started"
	if playErr := p.Start(c); playErr != nil {
		return "", playErr
	}

	return response, nil
}

// Adds the URL to the playlist and returns the corresponding answer or an error
func onAdd(p *player.Player, c *gumble.Client, words []string) (string, error) {
	if len(words) < 2 {
		return "", ErrTooFewArgs
	}

	regex := xurls.Strict()
	rawURL := regex.FindString(strings.Join(words[1:], " "))
	if rawURL == "" {
		return "", ErrNoURLFound
	}

	url, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	if err = p.AddToQueue(c, url); err != nil {
		return "", err
	}

	response := fmt.Sprintf("Added <a href=\"%s\">%s</a> to playlist", url.String(), url.String())
	return response, nil
}

// Stops the playlist and returns the corresponding answer or an error
func onStop(p *player.Player) (string, error) {
	if playErr := p.Stop(); playErr != nil {
		return "", playErr
	}

	return "Playlist stopped", nil
}

// Skips the song and returns the corresponding answer or an error
func onSkip(p *player.Player) (string, error) {
	if err := p.Skip(); err != nil {
		return "", err
	}

	return "Song skipped", nil
}

// Sets the volume and returns the corresponding answer or an error
func onVolume(p *player.Player, words []string) (string, error) {
	if len(words) < 2 {
		return "", ErrTooFewArgs
	}

	value, err := strconv.Atoi(words[1])
	if err != nil {
		return "", err
	}

	p.SetVolume(value)
	return fmt.Sprintf("Volume set to %d", value), err
}

// Receives an error and responds accordingly
// Returns true if the error is nil
func handleError(err error, c *gumble.Client) bool {
	if err == nil {
		return true
	}

	var response string
	switch {
	case errors.Is(err, player.ErrPlaying):
		response = "The playlist is already playing"
	case errors.Is(err, player.ErrEmpty):
		response = "The playlist is empty"
	case errors.Is(err, player.ErrStopped):
		response = "The playlist is already stopped"
	case errors.Is(err, player.ErrNoFormat):
		response = "Could not find correct format for song"
	case errors.Is(err, player.ErrVolumeRange):
		response = "The volume must be between 0 and 100"
	case errors.Is(err, ErrTooFewArgs):
		response = "Too few arguments given"
	case errors.Is(err, ErrNoURLFound):
		response = "Could not find URL"
	default:
		response = err.Error()
	}

	c.Self.Channel.Send(response, false)
	return false
}
