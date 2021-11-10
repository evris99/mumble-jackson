# Mumble Jackson
A music streaming bot for Mumble.

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