FROM golang:1.17-bullseye
ADD . /app
WORKDIR /app
RUN apt update
RUN apt install -y gcc ffmpeg
RUN go build -o mumble-jackson .
CMD [ "/app/mumble-jackson" ]