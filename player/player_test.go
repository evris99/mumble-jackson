package player

import (
	"net/url"
	"reflect"
	"testing"

	"layeh.com/gumble/gumble"
	"layeh.com/gumble/gumbleffmpeg"
	_ "layeh.com/gumble/opus"
)

func Test_getStream(t *testing.T) {
	type args struct {
		u      *url.URL
		client *gumble.Client
	}
	tests := []struct {
		name    string
		args    args
		want    *gumbleffmpeg.Stream
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getStream(tt.args.u, tt.args.client)
			if (err != nil) != tt.wantErr {
				t.Errorf("getStream() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getStream() = %v, want %v", got, tt.want)
			}
		})
	}
}
