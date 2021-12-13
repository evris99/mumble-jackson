# Mumble Jackson

A music streaming bot for Mumble.

[![API Reference](https://camo.githubusercontent.com/915b7be44ada53c290eb157634330494ebe3e30a/68747470733a2f2f676f646f632e6f72672f6769746875622e636f6d2f676f6c616e672f6764646f3f7374617475732e737667)](https://pkg.go.dev/github.com/evris99/mumble-music-bot)

## Installation

### Linux

First we need to install the dependencies. If you are in a Debian-based distro run:

```
sudo apt install libopus-dev gcc ffmpeg
```

Next we install go from https://golang.org/dl/.
Then run:

```
git clone https://github.com/evris99/mumble-jackson.git
cd mumble-jackson
go build .
./mumble-jackson -c configuration.toml
```
