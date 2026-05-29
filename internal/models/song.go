package models

// Song matches API JSON from the Perl server.
type Song struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Artist       string  `json:"artist"`
	Album        string  `json:"album"`
	Genre        string  `json:"genre"`
	Duration     *int    `json:"duration,omitempty"`
	Rating       int     `json:"rating"`
	Location     string  `json:"location"`
	TrackNumber  any     `json:"track_number,omitempty"`
	Bitrate      any     `json:"bitrate,omitempty"`
	IsRadio      bool    `json:"is_radio,omitempty"`
	AlbumArtist  string  `json:"album_artist,omitempty"`
	PlayCount    int     `json:"play_count,omitempty"`
	SkipCount    int     `json:"skip_count,omitempty"`
	Highlight    map[string]string `json:"highlight,omitempty"`
}

type FriendLink struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type LoadingStatus struct {
	Done      bool   `json:"done"`
	Total     int    `json:"total"`
	Processed int    `json:"processed"`
	Message   string `json:"message"`
	Progress  int    `json:"progress"`
}

type LibraryResult struct {
	Songs []Song `json:"songs"`
	Total int    `json:"total"`
}
