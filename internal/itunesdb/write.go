package itunesdb

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/BrandonKowalski/cw/internal/library"
)

type Track struct {
	ID          uint32
	DBID        uint64
	Path        string
	Tags        library.Tags
	DurationMS  int
	SampleRate  int
	BitrateKbps int
	Size        int64
	DateAdded   time.Time
	ArtworkID   uint32
	ArtworkSize uint32
}

const (
	macEpochOffset = 2082844800
	filetypeName   = "Apple Lossless audio file"
	masterName     = "iPod"
)

const (
	sortTitle    = 0x03
	sortAlbum    = 0x04
	sortArtist   = 0x05
	sortGenre    = 0x07
	sortComposer = 0x12
)

func MacTime(t time.Time) uint32 {
	s := t.Unix() + macEpochOffset
	if s < 0 {
		return 0
	}
	if s > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(s)
}

func trackTime(t time.Time) uint32 {
	if t.IsZero() {
		return 0
	}
	return MacTime(t)
}

func filetypeMarker(path string) uint32 {
	suffix := "."
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		suffix = path[i:]
	}
	var m uint32
	for i := 1; i <= 4; i++ {
		m <<= 8
		if len(suffix) > i {
			c := suffix[i]
			if c >= 'a' && c <= 'z' {
				c -= 'a' - 'A'
			}
			m |= uint32(c)
		} else {
			m |= ' '
		}
	}
	return m
}

func sqlID(kind string, parts ...string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(kind))
	for _, p := range parts {
		h.Write([]byte{0})
		h.Write([]byte(p))
	}
	if v := h.Sum64(); v != 0 {
		return v
	}
	return 1
}

type album struct {
	id    uint32
	first *Track
}

func sameAlbum(a, b *Track) bool {
	if a.Tags.Album != b.Tags.Album {
		return false
	}
	if a.Tags.AlbumArtist != "" && b.Tags.AlbumArtist != "" {
		return a.Tags.AlbumArtist == b.Tags.AlbumArtist
	}
	return a.Tags.Artist == b.Tags.Artist
}

type groups struct {
	albums      []*album
	albumByName map[string][]*album
	artists     []string
	artistIDs   map[string]uint32
	composerIDs map[string]uint32
	trackAlbum  []uint32
	trackArtist []uint32
	trackComp   []uint32
}

func groupTracks(tracks []Track) *groups {
	g := &groups{
		albumByName: map[string][]*album{},
		artistIDs:   map[string]uint32{},
		composerIDs: map[string]uint32{},
		trackAlbum:  make([]uint32, len(tracks)),
		trackArtist: make([]uint32, len(tracks)),
		trackComp:   make([]uint32, len(tracks)),
	}
	for i := range tracks {
		t := &tracks[i]
		if name := t.Tags.Album; name != "" {
			var found *album
			for _, a := range g.albumByName[name] {
				if sameAlbum(a.first, t) {
					found = a
					break
				}
			}
			if found == nil {
				found = &album{id: uint32(len(g.albums) + 1), first: t}
				g.albums = append(g.albums, found)
				g.albumByName[name] = append(g.albumByName[name], found)
			}
			g.trackAlbum[i] = found.id
		}
		if a := t.Tags.Artist; a != "" {
			id, ok := g.artistIDs[a]
			if !ok {
				g.artists = append(g.artists, a)
				id = uint32(len(g.artists))
				g.artistIDs[a] = id
			}
			g.trackArtist[i] = id
		}
		if c := t.Tags.Composer; c != "" {
			id, ok := g.composerIDs[c]
			if !ok {
				id = uint32(len(g.composerIDs) + 1)
				g.composerIDs[c] = id
			}
			g.trackComp[i] = id
		}
	}
	return g
}

func trackMhods(t *Track) [][]byte {
	var out [][]byte
	add := func(typ uint32, s string) {
		if s != "" {
			out = append(out, mhodString(typ, s))
		}
	}
	add(mhodTitle, t.Tags.Title)
	add(mhodArtist, t.Tags.Artist)
	add(mhodAlbum, t.Tags.Album)
	add(mhodFiletype, filetypeName)
	add(mhodPath, t.Path)
	add(mhodGenre, t.Tags.Genre)
	add(mhodComposer, t.Tags.Composer)
	add(mhodAlbumArtist, t.Tags.AlbumArtist)
	add(mhodSortArtist, t.Tags.SortArtist)
	add(mhodSortTitle, t.Tags.SortTitle)
	add(mhodSortAlbum, t.Tags.SortAlbum)
	add(mhodSortAlbumArtist, t.Tags.SortAlbumArtist)
	return out
}

func u32Field(name string, v int64) (uint32, error) {
	if v < 0 || v > math.MaxUint32 {
		return 0, fmt.Errorf("itunesdb: %s %d out of range", name, v)
	}
	return uint32(v), nil
}

func buildMhit(t *Track, g *groups, i int) ([]byte, error) {
	f := mhitFields{
		id:         t.ID,
		filetype:   filetypeMarker(t.Path),
		modified:   trackTime(t.DateAdded),
		added:      trackTime(t.DateAdded),
		dbid:       t.DBID,
		albumID:    g.trackAlbum[i],
		artistID:   g.trackArtist[i],
		composerID: g.trackComp[i],
		hasArtwork: 2,
	}
	if t.Tags.Compilation {
		f.compilation = 1
	}
	if t.ArtworkID != 0 {
		f.artworkCount = 1
		f.hasArtwork = 1
		f.mhiiLink = t.ArtworkID
		f.artworkSize = t.ArtworkSize
	}
	fields := []struct {
		name string
		v    int64
		dst  *uint32
	}{
		{"size", t.Size, &f.size},
		{"duration", int64(t.DurationMS), &f.lengthMS},
		{"track number", int64(t.Tags.TrackNumber), &f.trackNr},
		{"track total", int64(t.Tags.TrackTotal), &f.tracks},
		{"year", int64(t.Tags.Year), &f.year},
		{"bitrate", int64(t.BitrateKbps), &f.bitrate},
		{"disc number", int64(t.Tags.DiscNumber), &f.cdNr},
		{"disc total", int64(t.Tags.DiscTotal), &f.cds},
	}
	for _, fl := range fields {
		v, err := u32Field(fl.name, fl.v)
		if err != nil {
			return nil, fmt.Errorf("track %d: %w", t.ID, err)
		}
		*fl.dst = v
	}
	if t.SampleRate < 0 || t.SampleRate > math.MaxUint16 {
		return nil, fmt.Errorf("itunesdb: track %d: sample rate %d out of range", t.ID, t.SampleRate)
	}
	f.sampleRate = uint32(t.SampleRate)
	return mhit(f, trackMhods(t)), nil
}

type sortRec struct {
	index                                  uint32
	title, album, artist, genre, composer  string
	lTitle, lAlbum, lArtist, lGenre, lComp uint16
	trackNr, cdNr                          int
}

func sortArtistKey(t *Track) string {
	if t.Tags.SortArtist != "" {
		return t.Tags.SortArtist
	}
	a := t.Tags.Artist
	if len(a) >= 4 && strings.EqualFold(a[:4], "The ") {
		return a[4:] + ", The\x01\x01\x01\x01\x01"
	}
	return a
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func jumpLetter(s string) uint16 {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return utf16.Encode([]rune{unicode.ToUpper(r)})[0]
		}
		if unicode.IsNumber(r) {
			return '0'
		}
	}
	return '0'
}

func collate(s string) string {
	return strings.ToLower(s)
}

func newSortRecs(tracks []Track) []*sortRec {
	recs := make([]*sortRec, len(tracks))
	for i := range tracks {
		t := &tracks[i]
		title := firstNonEmpty(t.Tags.SortTitle, t.Tags.Title)
		alb := firstNonEmpty(t.Tags.SortAlbum, t.Tags.Album)
		art := sortArtistKey(t)
		recs[len(tracks)-1-i] = &sortRec{
			index:    uint32(i),
			title:    collate(title),
			album:    collate(alb),
			artist:   collate(art),
			genre:    collate(t.Tags.Genre),
			composer: collate(t.Tags.Composer),
			lTitle:   jumpLetter(title),
			lAlbum:   jumpLetter(alb),
			lArtist:  jumpLetter(art),
			lGenre:   jumpLetter(t.Tags.Genre),
			lComp:    jumpLetter(t.Tags.Composer),
			trackNr:  t.Tags.TrackNumber,
			cdNr:     t.Tags.DiscNumber,
		}
	}
	return recs
}

func cmpChain(cs ...int) int {
	for _, c := range cs {
		if c != 0 {
			return c
		}
	}
	return 0
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func albumTail(a, b *sortRec) []int {
	return []int{
		strings.Compare(a.album, b.album),
		cmpInt(a.cdNr, b.cdNr),
		cmpInt(a.trackNr, b.trackNr),
		strings.Compare(a.title, b.title),
	}
}

var sortOrders = []struct {
	typ    uint32
	cmp    func(a, b *sortRec) int
	letter func(r *sortRec) uint16
}{
	{sortTitle, func(a, b *sortRec) int { return strings.Compare(a.title, b.title) }, func(r *sortRec) uint16 { return r.lTitle }},
	{sortArtist, func(a, b *sortRec) int {
		return cmpChain(append([]int{strings.Compare(a.artist, b.artist)}, albumTail(a, b)...)...)
	}, func(r *sortRec) uint16 { return r.lArtist }},
	{sortAlbum, func(a, b *sortRec) int { return cmpChain(albumTail(a, b)...) }, func(r *sortRec) uint16 { return r.lAlbum }},
	{sortGenre, func(a, b *sortRec) int {
		return cmpChain(append([]int{strings.Compare(a.genre, b.genre), strings.Compare(a.artist, b.artist)}, albumTail(a, b)...)...)
	}, func(r *sortRec) uint16 { return r.lGenre }},
	{sortComposer, func(a, b *sortRec) int {
		return cmpChain(append([]int{strings.Compare(a.composer, b.composer)}, albumTail(a, b)...)...)
	}, func(r *sortRec) uint16 { return r.lComp }},
}

func libraryIndexMhods(tracks []Track) [][]byte {
	recs := newSortRecs(tracks)
	var out [][]byte
	for _, o := range sortOrders {
		sort.SliceStable(recs, func(i, j int) bool { return o.cmp(recs[i], recs[j]) < 0 })
		indices := make([]uint32, len(recs))
		var entries []jumpEntry
		var last uint16
		for i, r := range recs {
			indices[i] = r.index
			l := o.letter(r)
			if len(entries) == 0 || l != last {
				entries = append(entries, jumpEntry{letter: l, start: uint32(i)})
				last = l
			}
			entries[len(entries)-1].count++
		}
		out = append(out, mhodLibIndex(o.typ, indices), mhodJumpTable(o.typ, entries))
	}
	return out
}

func masterPlaylist(tracks []Track, dbID uint64) []byte {
	var ts uint32
	for i := range tracks {
		if v := trackTime(tracks[i].DateAdded); v > ts {
			ts = v
		}
	}
	var idb [8]byte
	binary.LittleEndian.PutUint64(idb[:], dbID)
	mhods := [][]byte{mhodString(mhodTitle, masterName), mhodPlaylistPrefs()}
	if len(tracks) > 0 {
		mhods = append(mhods, libraryIndexMhods(tracks)...)
	}
	mhips := make([][]byte, len(tracks))
	for i := range tracks {
		mhips[i] = mhip(tracks[i].ID, uint32(i))
	}
	return mhyp(mhypFields{
		master:    true,
		timestamp: ts,
		id:        sqlID("mpl", string(idb[:])),
		mhips:     len(tracks),
	}, mhods, mhips)
}

func albumList(g *groups) []byte {
	parts := [][]byte{listHeader("mhla", len(g.albums))}
	for _, a := range g.albums {
		t := a.first
		var mhods [][]byte
		if t.Tags.Album != "" {
			mhods = append(mhods, mhodString(mhodAlbumAlbum, t.Tags.Album))
		}
		if s := firstNonEmpty(t.Tags.AlbumArtist, t.Tags.Artist); s != "" {
			mhods = append(mhods, mhodString(mhodAlbumArtistName, s))
		}
		if s := firstNonEmpty(t.Tags.SortAlbumArtist, t.Tags.SortArtist); s != "" {
			mhods = append(mhods, mhodString(mhodAlbumSortArtist, s))
		}
		parts = append(parts, mhia(a.id, sqlID("album", t.Tags.Album, t.Tags.AlbumArtist, t.Tags.Artist), mhods))
	}
	return concat(parts)
}

func artistList(g *groups) []byte {
	parts := [][]byte{listHeader("mhli", len(g.artists))}
	for i, a := range g.artists {
		parts = append(parts, mhii(uint32(i+1), sqlID("artist", a), [][]byte{mhodString(mhodArtistName, a)}))
	}
	return concat(parts)
}

func Write(w io.Writer, tracks []Track, dbID uint64) error {
	g := groupTracks(tracks)
	items := [][]byte{listHeader("mhlt", len(tracks))}
	for i := range tracks {
		b, err := buildMhit(&tracks[i], g, i)
		if err != nil {
			return err
		}
		items = append(items, b)
	}
	mpl := masterPlaylist(tracks, dbID)
	playlists := concat([][]byte{listHeader("mhlp", 1), mpl})
	sds := [][]byte{
		mhsd(1, concat(items)),
		mhsd(3, playlists),
		mhsd(2, playlists),
		mhsd(4, albumList(g)),
		mhsd(8, artistList(g)),
		mhsd(6, listHeader("mhlt", 0)),
		mhsd(10, listHeader("mhlt", 0)),
		mhsd(5, listHeader("mhlp", 0)),
	}
	out := mhbd(mhbdFields{children: uint32(len(sds)), id: dbID, pid: dbID}, concat(sds))
	_, err := w.Write(out)
	return err
}
