package library

import (
	"strconv"
	"strings"
)

type Tags struct {
	Title           string `json:"title"`
	Artist          string `json:"artist"`
	Album           string `json:"album"`
	AlbumArtist     string `json:"album_artist"`
	Genre           string `json:"genre"`
	Composer        string `json:"composer"`
	Year            int    `json:"year"`
	TrackNumber     int    `json:"track_number"`
	TrackTotal      int    `json:"track_total"`
	DiscNumber      int    `json:"disc_number"`
	DiscTotal       int    `json:"disc_total"`
	Compilation     bool   `json:"compilation"`
	SortTitle       string `json:"sort_title"`
	SortArtist      string `json:"sort_artist"`
	SortAlbum       string `json:"sort_album"`
	SortAlbumArtist string `json:"sort_album_artist"`
}

func join(m map[string][]string, key string) string {
	var out []string
	for _, v := range m[key] {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return strings.Join(out, ", ")
}

func first(m map[string][]string, keys ...string) string {
	for _, k := range keys {
		for _, v := range m[k] {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
	}
	return ""
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func numTotal(s string) (int, int) {
	n, t, _ := strings.Cut(s, "/")
	return atoi(n), atoi(t)
}

func year(m map[string][]string) int {
	for _, k := range []string{"DATE", "ORIGINALYEAR"} {
		s := first(m, k)
		if len(s) >= 4 {
			if y, err := strconv.Atoi(s[:4]); err == nil {
				return y
			}
		}
	}
	return 0
}

func TagsFromVorbis(m map[string][]string, fileStem string) Tags {
	t := Tags{
		Title:           join(m, "TITLE"),
		Artist:          join(m, "ARTIST"),
		Album:           join(m, "ALBUM"),
		AlbumArtist:     join(m, "ALBUMARTIST"),
		Genre:           join(m, "GENRE"),
		Composer:        join(m, "COMPOSER"),
		Year:            year(m),
		Compilation:     first(m, "COMPILATION") == "1",
		SortTitle:       join(m, "TITLESORT"),
		SortArtist:      join(m, "ARTISTSORT"),
		SortAlbum:       join(m, "ALBUMSORT"),
		SortAlbumArtist: join(m, "ALBUMARTISTSORT"),
	}
	if t.AlbumArtist == "" {
		t.AlbumArtist = t.Artist
	}
	if t.Title == "" {
		t.Title = fileStem
	}
	var tt, dt int
	t.TrackNumber, tt = numTotal(first(m, "TRACKNUMBER"))
	t.DiscNumber, dt = numTotal(first(m, "DISCNUMBER"))
	if v := atoi(first(m, "TRACKTOTAL", "TOTALTRACKS")); v > 0 {
		tt = v
	}
	if v := atoi(first(m, "DISCTOTAL", "TOTALDISCS")); v > 0 {
		dt = v
	}
	t.TrackTotal, t.DiscTotal = tt, dt
	return t
}
