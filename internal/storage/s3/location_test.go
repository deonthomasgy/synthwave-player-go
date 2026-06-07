package s3

import "testing"

func TestParseURI(t *testing.T) {
	loc, err := ParseURI("s3://music/Artist/song.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if loc.Bucket != "music" || loc.Key != "Artist/song.mp3" {
		t.Fatalf("got %+v", loc)
	}
	if loc.String() != "s3://music/Artist/song.mp3" {
		t.Fatalf("string %q", loc.String())
	}
}

func TestAllowed(t *testing.T) {
	loc := Location{Bucket: "music", Key: "Artist/song.mp3"}
	if !Allowed(loc, []string{"s3://music/"}) {
		t.Fatal("expected allowed under bucket root")
	}
	if Allowed(loc, []string{"s3://other/"}) {
		t.Fatal("expected denied for other bucket")
	}
	prefix := Location{Bucket: "music", Key: "live/set.mp3"}
	if !Allowed(prefix, []string{"s3://music/live/"}) {
		t.Fatal("expected allowed under prefix")
	}
	if Allowed(prefix, []string{"s3://music/archive/"}) {
		t.Fatal("expected denied outside prefix")
	}
}
