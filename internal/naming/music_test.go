package naming

import "testing"

func TestParseTrack(t *testing.T) {
	cases := []struct {
		path string
		want Track
	}{
		{"Music/Daft Punk/Discovery (2001)/01 - One More Time.flac", Track{Artist: "Daft Punk", Album: "Discovery", Title: "One More Time", Number: 1, Year: 2001}},
		{"Music/Daft Punk/Discovery (2001)/CD2/03 - Title.flac", Track{Artist: "Daft Punk", Album: "Discovery", Title: "Title", Disc: 2, Number: 3, Year: 2001}},
		{"Music/Artist/Album (2010)/Disc 1/05. Song.mp3", Track{Artist: "Artist", Album: "Album", Title: "Song", Disc: 1, Number: 5, Year: 2010}},
		{"Music/Artist/Album/01. Title.mp3", Track{Artist: "Artist", Album: "Album", Title: "Title", Number: 1}},
		{"01. Title.mp3", Track{Title: "Title", Number: 1}},
		{"1-03 Title.m4a", Track{Title: "Title", Disc: 1, Number: 3}},
		{"2-11 Another Song.m4a", Track{Title: "Another Song", Disc: 2, Number: 11}},
		{"Artist - Album - 03 - Title.mp3", Track{Artist: "Artist", Album: "Album", Title: "Title", Number: 3}},
		{"Music/Pink Floyd/The Wall (1979)/Pink Floyd - The Wall - 05 - Another Brick in the Wall, Part 2.flac",
			Track{Artist: "Pink Floyd", Album: "The Wall", Title: "Another Brick in the Wall, Part 2", Number: 5, Year: 1979}},
		{"Artist - 07 - Title.ogg", Track{Artist: "Artist", Title: "Title", Number: 7}},
		{"Artist - Title.mp3", Track{Artist: "Artist", Title: "Title"}},
		{"Music/Artist/Album/03.flac", Track{Artist: "Artist", Album: "Album", Number: 3}},
		{"Music/Artist/Album/Song Without Number.flac", Track{Artist: "Artist", Album: "Album", Title: "Song Without Number"}},
		{"Music/Artist/Album/Artist - Song.flac", Track{Artist: "Artist", Album: "Album", Title: "Song"}},
		{"Music/Artist/Album/Other - Song.flac", Track{Artist: "Artist", Album: "Album", Title: "Other - Song"}},
		{"Music/Artist/Album/01 - Artist - Song.flac", Track{Artist: "Artist", Album: "Album", Title: "Song", Number: 1}},
		{"Music/Artist/Album/01 - Song - Live.flac", Track{Artist: "Artist", Album: "Album", Title: "Song - Live", Number: 1}},
		{"Music/Daft Punk - Discovery (2001)/01 - One More Time.flac", Track{Artist: "Daft Punk", Album: "Discovery", Title: "One More Time", Number: 1, Year: 2001}},
		{"Music/Daft Punk/2001 - Discovery/01 - One More Time.flac", Track{Artist: "Daft Punk", Album: "Discovery", Title: "One More Time", Number: 1, Year: 2001}},
		{"Music/Daft Punk/[2001] Discovery/01 - One More Time.flac", Track{Artist: "Daft Punk", Album: "Discovery", Title: "One More Time", Number: 1, Year: 2001}},
		{"Music/Daft Punk/Daft Punk - 2001 - Discovery/01 - One More Time.flac", Track{Artist: "Daft Punk", Album: "Discovery", Title: "One More Time", Number: 1, Year: 2001}},
		{"Music/Daft Punk/Discovery (2001) [FLAC]/01 - One More Time.flac", Track{Artist: "Daft Punk", Album: "Discovery", Title: "One More Time", Number: 1, Year: 2001}},
		{"Music/Daft Punk/Discovery [2001] [24bit-96kHz]/01 - One More Time.flac", Track{Artist: "Daft Punk", Album: "Discovery", Title: "One More Time", Number: 1, Year: 2001}},
		{"Music/Michael Jackson/Bad (2012 Remaster)/01 - Bad.flac", Track{Artist: "Michael Jackson", Album: "Bad (2012 Remaster)", Title: "Bad", Number: 1}},
		{`D:\Musique\Stromae\Racine carrée (2013)\02 - Ta fête.flac`, Track{Artist: "Stromae", Album: "Racine carrée", Title: "Ta fête", Number: 2, Year: 2013}},
		{"Musique/Édith Piaf/La Vie en rose/01_La_Vie_en_rose.mp3", Track{Artist: "Édith Piaf", Album: "La Vie en rose", Title: "La Vie en rose", Number: 1}},
		{"Music/Artist/Album (2010)/CD 01/Artist - Album - 01 - Title.flac", Track{Artist: "Artist", Album: "Album", Title: "Title", Disc: 1, Number: 1, Year: 2010}},
		{"Music/Artist/Album/Disc04 (Encores and Folk Songs)/01 Title.flac", Track{Artist: "Artist", Album: "Album", Title: "Title", Disc: 4, Number: 1}},
		{"Music/R.E.M/Automatic for the People (1992)/02 Try Not to Breathe.mp3", Track{Artist: "R.E.M", Album: "Automatic for the People", Title: "Try Not to Breathe", Number: 2, Year: 1992}},
		{"Music/Weezer/Pinkerton/03 Pink Triangle.mp3", Track{Artist: "Weezer", Album: "Pinkerton", Title: "Pink Triangle", Number: 3}},
		{"Music/Artist/Album/(04) Title.mp3", Track{Artist: "Artist", Album: "Album", Title: "Title", Number: 4}},
		{"Music/Artist/Album/12-Title.mp3", Track{Artist: "Artist", Album: "Album", Title: "Title", Number: 12}},
		{"Artist - 1-03 - Title.flac", Track{Artist: "Artist", Title: "Title", Disc: 1, Number: 3}},
		{"", Track{}},
	}
	for _, tc := range cases {
		if got := ParseTrack(tc.path); got != tc.want {
			t.Errorf("ParseTrack(%q)\n got  %+v\n want %+v", tc.path, got, tc.want)
		}
	}
}
