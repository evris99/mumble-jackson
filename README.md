# Mumble Jackson

A music streaming bot for Mumble.

[![API Reference](https://camo.githubusercontent.com/915b7be44ada53c290eb157634330494ebe3e30a/68747470733a2f2f676f646f632e6f72672f6769746875622e636f6d2f676f6c616e672f6764646f3f7374617475732e737667)](https://pkg.go.dev/github.com/evris99/mumble-music-bot)

## Installation

### Docker

In order to build the docker container you must first create a configuration.toml file. To do this you can copy the existing example configuration.

```
cp configuration.example.toml configuration.toml
```

Then you need to make the necessary changes to the configuration.

To build the container run:

```
docker build -t mumble-jackson .
```

To run the container run:

```
docker run mumble-jackson
```

### Linux

To compile from source you need to have go installed.
First you need to install the dependencies. If you are in a Debian-based distro run:

```
sudo apt install libopus-dev gcc ffmpeg
```

Then to build the executable run:

```
git clone https://github.com/evris99/mumble-jackson.git
cd mumble-jackson
go build .
```

Rename the example configuration file.

```
cp configuration.example.toml configuration.toml
```

Then make the necessary changes to the file.

To start execute:

```
./mumble-jackson -c configuration.toml
```
