package naming

import (
	"reflect"
	"testing"
)

func eps(n ...int) []int { return n }

var episodeCases = []struct {
	path string
	want Episode
}{
	// Sonarr standard format
	{"TV/Breaking Bad (2008) {tvdb-81189}/Season 01/Breaking Bad (2008) - S01E01 - Pilot [HDTV-720p][x264]-GRP.mkv",
		Episode{SeriesTitle: "Breaking Bad", Season: 1, Episodes: eps(1), Title: "Pilot", Year: 2008, IDs: ProviderIDs{TVDB: "81189"}}},
	{"TV/Breaking Bad (2008) {tvdb-81189}/Season 01/Breaking Bad - S01E02 - Cat's in the Bag....mkv",
		Episode{SeriesTitle: "Breaking Bad", Season: 1, Episodes: eps(2), Title: "Cat's in the Bag...", Year: 2008, IDs: ProviderIDs{TVDB: "81189"}}},
	{"TV/The Office (US)/Season 2/The Office (US) - S02E01 - The Dundies.mkv",
		Episode{SeriesTitle: "The Office (US)", Season: 2, Episodes: eps(1), Title: "The Dundies"}},
	{"Series Title (2010) - S01E01-E03 - Episode Title [WEBDL-1080p].mkv",
		Episode{SeriesTitle: "Series Title", Season: 1, Episodes: eps(1, 2, 3), Title: "Episode Title", Year: 2010}},
	{"Series Title (2010) - S01E01-E02-E03 - Episode Title.mkv",
		Episode{SeriesTitle: "Series Title", Season: 1, Episodes: eps(1, 2, 3), Title: "Episode Title", Year: 2010}},
	{"Series Title - S01E01E02 - Title (1) + Title (2).mkv",
		Episode{SeriesTitle: "Series Title", Season: 1, Episodes: eps(1, 2), Title: "Title (1) + Title (2)"}},
	{"TV/Show/Specials/Show - S00E01 - Christmas Special.mkv",
		Episode{SeriesTitle: "Show", Season: 0, Episodes: eps(1), Title: "Christmas Special"}},
	{"TV/Show/Season 00/Show - S00E05.mkv",
		Episode{SeriesTitle: "Show", Season: 0, Episodes: eps(5)}},

	// Scene releases
	{"The.Simpsons.S25E09.Steal.this.episode.mp4",
		Episode{SeriesTitle: "The Simpsons", Season: 25, Episodes: eps(9), Title: "Steal this episode"}},
	{"Game.of.Thrones.S08E06.1080p.WEB.H264-MEMENTO.mkv",
		Episode{SeriesTitle: "Game of Thrones", Season: 8, Episodes: eps(6)}},
	{"Marvels.Agents.of.S.H.I.E.L.D.S01E01.720p.HDTV.x264-KILLERS.mkv",
		Episode{SeriesTitle: "Marvels Agents of S.H.I.E.L.D.", Season: 1, Episodes: eps(1)}},
	{"Mr.Robot.S02E01.eps2.0_unm4sk-pt1.tc.720p.WEB-DL.mkv",
		Episode{SeriesTitle: "Mr. Robot", Season: 2, Episodes: eps(1), Title: "eps2.0 unm4sk-pt1 tc"}},
	{"Doctor.Who.2005.S01E01.Rose.720p.BluRay.x264.mkv",
		Episode{SeriesTitle: "Doctor Who", Season: 1, Episodes: eps(1), Title: "Rose", Year: 2005}},
	{"The.Wonder.Years/The.Wonder.Years.S04.PDTV.x264-JCH/The Wonder Years s04e07 Christmas Party NTSC PDTV.avi",
		Episode{SeriesTitle: "The Wonder Years", Season: 4, Episodes: eps(7), Title: "Christmas Party"}},
	{"Running Man/Running Man S2017E368.mkv",
		Episode{SeriesTitle: "Running Man", Season: 2017, Episodes: eps(368)}},
	{"series-s09e14-1080p.mkv", Episode{SeriesTitle: "series", Season: 9, Episodes: eps(14)}},
	{"series-s09e14-720i.mkv", Episode{SeriesTitle: "series", Season: 9, Episodes: eps(14)}},
	{"MOONLIGHTING_s01e01-e04.mkv", Episode{SeriesTitle: "MOONLIGHTING", Season: 1, Episodes: eps(1, 2, 3, 4)}},
	{"Show.Name.S01E01E02.720p.mkv", Episode{SeriesTitle: "Show Name", Season: 1, Episodes: eps(1, 2)}},
	{"Show.Name.S01E01-02.720p.mkv", Episode{SeriesTitle: "Show Name", Season: 1, Episodes: eps(1, 2)}},
	{"Show.Name.S01E01-03.720p.mkv", Episode{SeriesTitle: "Show Name", Season: 1, Episodes: eps(1, 2, 3)}},
	{"Show.Name.S03E10.FRENCH.720p.HDTV.x264-GRP.mkv", Episode{SeriesTitle: "Show Name", Season: 3, Episodes: eps(10)}},
	{"Show.Name.S03E10.VOSTFR.WEBRip.mkv", Episode{SeriesTitle: "Show Name", Season: 3, Episodes: eps(10)}},
	{"Show Name S01 E02 Title.mkv", Episode{SeriesTitle: "Show Name", Season: 1, Episodes: eps(2), Title: "Title"}},
	{"Show Name s01.e02.mkv", Episode{SeriesTitle: "Show Name", Season: 1, Episodes: eps(2)}},
	{"Show Name S01EP02.mkv", Episode{SeriesTitle: "Show Name", Season: 1, Episodes: eps(2)}},
	{"S.W.A.T.2017.S05E01.720p.mkv", Episode{SeriesTitle: "S.W.A.T.", Season: 5, Episodes: eps(1), Year: 2017}},
	{"Show.S01E01.x264.mkv", Episode{SeriesTitle: "Show", Season: 1, Episodes: eps(1)}},

	// Jellyfin corpus: season folder + various numberings
	{"Season 02/S02E03 blah.avi", Episode{Season: 2, Episodes: eps(3), Title: "blah"}},
	{"Season 1/01x02 blah.avi", Episode{Season: 1, Episodes: eps(2), Title: "blah"}},
	{"Season 1/S01x02 blah.avi", Episode{Season: 1, Episodes: eps(2), Title: "blah"}},
	{"Season 1/S01E02 blah.avi", Episode{Season: 1, Episodes: eps(2), Title: "blah"}},
	{"Season 1/S01xE02 blah.avi", Episode{Season: 1, Episodes: eps(2), Title: "blah"}},
	{"Season 1/seriesname S01E02 blah.avi", Episode{SeriesTitle: "seriesname", Season: 1, Episodes: eps(2), Title: "blah"}},
	{"Season 1/seriesname 01x02 blah.avi", Episode{SeriesTitle: "seriesname", Season: 1, Episodes: eps(2), Title: "blah"}},
	{"Season 1/seriesname S01x02 blah.avi", Episode{SeriesTitle: "seriesname", Season: 1, Episodes: eps(2), Title: "blah"}},
	{"Season 1/seriesname S01xE02 blah.avi", Episode{SeriesTitle: "seriesname", Season: 1, Episodes: eps(2), Title: "blah"}},
	{"Season 2/02x03 - 02x04 - 02x15 - Ep Name.mp4", Episode{Season: 2, Episodes: eps(3, 4, 15), Title: "Ep Name"}},
	{"Season 02/02x03 - x04 - x15 - Ep Name.mp4", Episode{Season: 2, Episodes: eps(3, 4, 15), Title: "Ep Name"}},
	{"Season 2/Elementary - 02x03-04-15 - Ep Name.mp4", Episode{SeriesTitle: "Elementary", Season: 2, Episodes: eps(3, 4, 15), Title: "Ep Name"}},
	{"Season 02/Elementary - 02x03x04x15 - Ep Name.mp4", Episode{SeriesTitle: "Elementary", Season: 2, Episodes: eps(3, 4, 15), Title: "Ep Name"}},
	{"Season 02/02x03-E15 - Ep Name.mp4", Episode{Season: 2, Episodes: eps(3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15), Title: "Ep Name"}},
	{"Season 2/02x03 - 04 Ep Name.mp4", Episode{Season: 2, Episodes: eps(3), Title: "04 Ep Name"}},
	{"Season 1/Elementary - S01E23-E24-E26 - The Woman.mp4", Episode{SeriesTitle: "Elementary", Season: 1, Episodes: eps(23, 24, 26), Title: "The Woman"}},
	{"Season 2009/S2009E23-E24-E26 - The Woman.mp4", Episode{Season: 2009, Episodes: eps(23, 24, 26), Title: "The Woman"}},
	{"Season 2009/2009x02 blah.avi", Episode{Season: 2009, Episodes: eps(2), Title: "blah"}},
	{"Season 2009/Elementary - 2009x03 - 2009x04 - 2009x15 - Ep Name.mp4", Episode{SeriesTitle: "Elementary", Season: 2009, Episodes: eps(3, 4, 15), Title: "Ep Name"}},
	{"Season 1/02 - blah-02 a.avi", Episode{Season: 1, Episodes: eps(2), Title: "blah-02 a"}},
	{"Season 1/02 - blah.avi", Episode{Season: 1, Episodes: eps(2), Title: "blah"}},
	{"Season 2/02 - blah 14 blah.avi", Episode{Season: 2, Episodes: eps(2), Title: "blah 14 blah"}},
	{"Season 2/02.avi", Episode{Season: 2, Episodes: eps(2)}},
	{"Season 2/2. Infestation.avi", Episode{Season: 2, Episodes: eps(2), Title: "Infestation"}},
	{"Season 1/02-03 - blah.avi", Episode{Season: 1, Episodes: eps(2, 3), Title: "blah"}},
	{"Season 2/02-04 - blah 14 blah.avi", Episode{Season: 2, Episodes: eps(2, 3, 4), Title: "blah 14 blah"}},
	{"Season 2/02-04.avi", Episode{Season: 2, Episodes: eps(2, 3, 4)}},
	{"Season 2/Episode - 16.avi", Episode{Season: 2, Episodes: eps(16)}},
	{"Season 2/Episode 16.avi", Episode{Season: 2, Episodes: eps(16)}},
	{"Season 2/Episode 16 - Some Title.avi", Episode{Season: 2, Episodes: eps(16), Title: "Some Title"}},
	{"Season 2/16 Some Title.avi", Episode{Season: 2, Episodes: eps(16), Title: "Some Title"}},
	{"Season 2/16 - 12 Some Title.avi", Episode{Season: 2, Episodes: eps(16), Title: "12 Some Title"}},
	{"Season 2/7 - 12 Angry Men.avi", Episode{Season: 2, Episodes: eps(7), Title: "12 Angry Men"}},
	{"Season 2/Episode 21 - 94 Meetings.mp4", Episode{Season: 2, Episodes: eps(21), Title: "94 Meetings"}},
	{"Season 3/The Series Season 3 Episode 9 - The title.avi", Episode{SeriesTitle: "The Series", Season: 3, Episodes: eps(9), Title: "The title"}},
	{"Season 3/The Series S3 E9 - The title.avi", Episode{SeriesTitle: "The Series", Season: 3, Episodes: eps(9), Title: "The title"}},
	{"Season 3/S003 E009.avi", Episode{Season: 3, Episodes: eps(9)}},
	{"Season 3/Season 3 Episode 9.avi", Episode{Season: 3, Episodes: eps(9)}},
	{"Season 1/4x01 – 20 Hours in America (1).mkv", Episode{Season: 4, Episodes: eps(1), Title: "20 Hours in America (1)"}},
	{"Season 1/S01E01 The 6-10 to Lubbock [WEBRip-1080p][AV1 Opus].mkv", Episode{Season: 1, Episodes: eps(1), Title: "The 6-10 to Lubbock"}},
	{"Season 5/S05E23 11-59 [HDTV-1080p][x265 AC3].mkv", Episode{Season: 5, Episodes: eps(23), Title: "11-59"}},
	{"Season 1/S01E01 1-23-45 [Bluray-1080p][AV1 Opus].mkv", Episode{Season: 1, Episodes: eps(1), Title: "1-23-45"}},
	{"Season 03/Star Trek Enterprise (2001) - S03E21 - E2 (1080p BluRay x265).mkv", Episode{SeriesTitle: "Star Trek Enterprise", Season: 3, Episodes: eps(21), Title: "E2", Year: 2001}},
	{"Season 02/Series Name (2001) - S02E10 - E5 [WEBRip-1080p].mkv", Episode{SeriesTitle: "Series Name", Season: 2, Episodes: eps(10), Title: "E5", Year: 2001}},
	{"Watchmen (2019)/Watchmen 1x03 - She Was Killed by Space Junk [WEBDL-720p][EAC3 5.1][h264][-TBS].mkv",
		Episode{SeriesTitle: "Watchmen", Season: 1, Episodes: eps(3), Title: "She Was Killed by Space Junk", Year: 2019}},
	{"Castle Rock 2x01 Que el rio siga su curso [WEB-DL HULU 1080p h264 Dual DD5.1 Subs].mkv",
		Episode{SeriesTitle: "Castle Rock", Season: 2, Episodes: eps(1), Title: "Que el rio siga su curso"}},
	{"/The.Legend.of.Condor.Heroes.2017.V2.web-dl.1080p.h264.aac-hdctv/The.Legend.of.Condor.Heroes.2017.E07.V2.web-dl.1080p.h264.aac-hdctv.mkv",
		Episode{SeriesTitle: "The Legend of Condor Heroes", Season: -1, Episodes: eps(7), Title: "V2", Year: 2017}},
	{"The Simpsons/Foo_ep_02.avi", Episode{SeriesTitle: "Foo", Season: -1, Episodes: eps(2)}},
	{"The Simpsons/12.avi", Episode{SeriesTitle: "The Simpsons", Season: -1, Episodes: eps(12)}},
	{"The Simpsons/The Simpsons 12.avi", Episode{SeriesTitle: "The Simpsons", Season: -1, Episodes: eps(12), Absolute: 12}},
	{"The Simpsons/The Simpsons 112.avi", Episode{SeriesTitle: "The Simpsons", Season: -1, Episodes: eps(112), Absolute: 112}},
	{"Season 1/seriesname 05.mkv", Episode{SeriesTitle: "seriesname", Season: 1, Episodes: eps(5), Absolute: 5}},

	// Series title from folders
	{"TV/Breaking Bad/Season 1/S01E01.mkv", Episode{SeriesTitle: "Breaking Bad", Season: 1, Episodes: eps(1)}},
	{`D:\TV\Breaking Bad (2008)\Season 1\S01E01.mkv`, Episode{SeriesTitle: "Breaking Bad", Season: 1, Episodes: eps(1), Year: 2008}},
	{"TV/Show [tvdbid-12345]/S01/1x05.mkv", Episode{SeriesTitle: "Show", Season: 1, Episodes: eps(5), IDs: ProviderIDs{TVDB: "12345"}}},
	{"TV/Show/Saison 2/Show - S02E05 - Le Titre.mkv", Episode{SeriesTitle: "Show", Season: 2, Episodes: eps(5), Title: "Le Titre"}},
	{"TV/Show/Staffel 3/Episode 4.mkv", Episode{SeriesTitle: "Show", Season: 3, Episodes: eps(4)}},
	{"Séries/Le Bureau des légendes (2015)/Saison 01/Le.Bureau.des.Legendes.S01E01.FRENCH.720p.mkv",
		Episode{SeriesTitle: "Le Bureau des Legendes", Season: 1, Episodes: eps(1), Year: 2015}},
	{"Séries/Kaamelott/Livre 1/Kaamelott - S01E01 - Heat.mkv", Episode{SeriesTitle: "Kaamelott", Season: 1, Episodes: eps(1), Title: "Heat"}},
	{"TV/Show {imdb-tt0903747}/Season 1/Show S01E01 {tvdb-349232}.mkv", Episode{SeriesTitle: "Show", Season: 1, Episodes: eps(1), IDs: ProviderIDs{TVDB: "349232", IMDB: "tt0903747"}}},
	{"TV/Show/Season 1/Extras/Show S01E01 behind.mkv", Episode{SeriesTitle: "Show", Season: 1, Episodes: eps(1), Title: "behind"}},

	// Daily shows
	{"/server/anything_1996.11.14.mp4", Episode{SeriesTitle: "anything", Season: -1, AirDate: "1996-11-14"}},
	{"/server/anything_1996-11-14.mp4", Episode{SeriesTitle: "anything", Season: -1, AirDate: "1996-11-14"}},
	{"/server/james.corden.2017.04.20.anne.hathaway.720p.hdtv.x264-crooks.mkv",
		Episode{SeriesTitle: "james corden", Season: -1, AirDate: "2017-04-20", Title: "anne hathaway"}},
	{"/server/ABC News 2018_03_24_19_00_00.mkv", Episode{SeriesTitle: "ABC News", Season: -1, AirDate: "2018-03-24"}},
	{"/server/Jeopardy 2023 07 14 HDTV x264 AC3.mkv", Episode{SeriesTitle: "Jeopardy", Season: -1, AirDate: "2023-07-14"}},
	{"/server/anything_14.11.1996.mp4", Episode{SeriesTitle: "anything", Season: -1, AirDate: "1996-11-14"}},
	{"/server/anything_11-14-1996.mp4", Episode{SeriesTitle: "anything", Season: -1, AirDate: "1996-11-14"}},
	{"/server/A Daily Show - (2015-01-15) - Episode Name - [720p].mkv", Episode{SeriesTitle: "A Daily Show", Season: -1, AirDate: "2015-01-15", Title: "Episode Name"}},
	{"The Daily Show/Season 2024/The Daily Show - 2024-03-15 - Guest Name [WEBDL-1080p].mkv",
		Episode{SeriesTitle: "The Daily Show", Season: 2024, AirDate: "2024-03-15", Title: "Guest Name"}},
	{"Le Journal de 20h/Le.Journal.de.20h.15.03.2024.FRENCH.mkv", Episode{SeriesTitle: "Le Journal de 20h", Season: -1, AirDate: "2024-03-15"}},
	{"Show/Show 2024.02.30.mkv", Episode{}},
	{"Show/Show 03.04.2024.mkv", Episode{}},

	// Anime / absolute numbering
	{"Season 2/[HorribleSubs] Hunter X Hunter - 136 [720p].mkv", Episode{SeriesTitle: "Hunter X Hunter", Season: 2, Episodes: eps(136), Absolute: 136}},
	{"Log Horizon 2/[HorribleSubs] Log Horizon 2 - 03 [720p].mkv", Episode{SeriesTitle: "Log Horizon 2", Season: -1, Episodes: eps(3), Absolute: 3}},
	{"[BBT-RMX] Ranma ½ - 154 [50AC421A].mkv", Episode{SeriesTitle: "Ranma ½", Season: -1, Episodes: eps(154), Absolute: 154}},
	{"[VCB-Studio] Re Zero kara Hajimeru Isekai Seikatsu [21][Ma10p_1080p][x265_flac].mkv",
		Episode{SeriesTitle: "Re Zero kara Hajimeru Isekai Seikatsu", Season: -1, Episodes: eps(21), Absolute: 21}},
	{"[CASO&Sumisora][Oda_Nobuna_no_Yabou][04][BDRIP][1920x1080][x264_AAC][7620E503].mp4",
		Episode{SeriesTitle: "Oda Nobuna no Yabou", Season: -1, Episodes: eps(4), Absolute: 4}},
	{"Case Closed (1996-2007)/Case Closed - 317.mkv", Episode{SeriesTitle: "Case Closed", Season: -1, Episodes: eps(317), Absolute: 317, Year: 1996}},
	{"Season 2/Hunter X Hunter - 101.mkv", Episode{SeriesTitle: "Hunter X Hunter", Season: 2, Episodes: eps(101), Absolute: 101}},
	{"Season 1/Show Name - 1234 [720p].mkv", Episode{SeriesTitle: "Show Name", Season: 1, Episodes: eps(1234), Absolute: 1234}},
	{"Season 21/One Piece 1001.mkv", Episode{SeriesTitle: "One Piece", Season: 21, Episodes: eps(1001), Absolute: 1001}},
	{"[Erai-raws] Jujutsu Kaisen - 03 [720p][Multiple Subtitle].mkv", Episode{SeriesTitle: "Jujutsu Kaisen", Season: -1, Episodes: eps(3), Absolute: 3}},
	{"[SubsPlease] Frieren - 12v2 (1080p) [ABCD1234].mkv", Episode{SeriesTitle: "Frieren", Season: -1, Episodes: eps(12), Absolute: 12}},
	{"Anime/One Piece/[Group] One Piece - 1071 [1080p].mkv", Episode{SeriesTitle: "One Piece", Season: -1, Episodes: eps(1071), Absolute: 1071}},
	{"[Judas] Shingeki no Kyojin - S04E28 [1080p][HEVC x265 10bit].mkv", Episode{SeriesTitle: "Shingeki no Kyojin", Season: 4, Episodes: eps(28)}},
	{"Naruto Shippuden - 500 - The Message.mkv", Episode{SeriesTitle: "Naruto Shippuden", Season: -1, Episodes: eps(500), Absolute: 500, Title: "The Message"}},

	{"Anime/Frieren (2023)/Season 1/Frieren (2023) - S01E05 - 005 - Phantoms of the Dead [WEBDL-1080p].mkv",
		Episode{SeriesTitle: "Frieren", Season: 1, Episodes: eps(5), Absolute: 5, Title: "Phantoms of the Dead", Year: 2023}},
	{"Dragon Ball Z - 001 - The New Threat.mkv", Episode{SeriesTitle: "Dragon Ball Z", Season: -1, Episodes: eps(1), Absolute: 1, Title: "The New Threat"}},

	// More real-world names
	{"TV/Show/Season 1/Show - 1x01 - Pilot.mkv", Episode{SeriesTitle: "Show", Season: 1, Episodes: eps(1), Title: "Pilot"}},
	{"TV/Show/Season 1/Show - Episode 1 - Pilot.mkv", Episode{SeriesTitle: "Show", Season: 1, Episodes: eps(1), Title: "Pilot"}},
	{"TV/Show (2010)/Season 1/Show (2010) - S01E01 - Pilot (1080p).mkv", Episode{SeriesTitle: "Show", Season: 1, Episodes: eps(1), Title: "Pilot", Year: 2010}},
	{"La Casa de Papel - 1x01 - Efectuar lo acordado.mkv", Episode{SeriesTitle: "La Casa de Papel", Season: 1, Episodes: eps(1), Title: "Efectuar lo acordado"}},
	{"TV/Bluey (2018)/Season 1/Bluey (2018) - S01E01 - Magic Xylophone [WEBDL-1080p][EAC3 2.0][h264]-NTb.mkv",
		Episode{SeriesTitle: "Bluey", Season: 1, Episodes: eps(1), Title: "Magic Xylophone", Year: 2018}},
	{"Lupin.S01E01.MULTi.1080p.WEB.x264-GRP.mkv", Episode{SeriesTitle: "Lupin", Season: 1, Episodes: eps(1)}},
	{"The.Office.US.S01E01.mkv", Episode{SeriesTitle: "The Office US", Season: 1, Episodes: eps(1)}},
	{"Séries/Dix pour cent/Saison 1/Dix pour cent - Épisode 3.mkv", Episode{SeriesTitle: "Dix pour cent", Season: 1, Episodes: eps(3)}},
	{"Séries/Engrenages/Saison 2/Engrenages.S02E03.VFF.HDTV.XviD-GRP.avi", Episode{SeriesTitle: "Engrenages", Season: 2, Episodes: eps(3)}},
	{"TV/Show/Season 1/Show.S01E01.Pilot.PROPER.720p.mkv", Episode{SeriesTitle: "Show", Season: 1, Episodes: eps(1), Title: "Pilot"}},
	{"TV/Show/Season 1/Show S01E01 & E02.mkv", Episode{SeriesTitle: "Show", Season: 1, Episodes: eps(1, 2)}},
	{"TV/Show/Season 1/Show - S01E01 - S01E02 - Title.mkv", Episode{SeriesTitle: "Show", Season: 1, Episodes: eps(1, 2), Title: "Title"}},
	{"TV/Show/Season 1/Show.1x01-1x02.mkv", Episode{SeriesTitle: "Show", Season: 1, Episodes: eps(1, 2)}},
	{"TV/Show/Season 1/Show.S01E01-E50.mkv", Episode{SeriesTitle: "Show", Season: 1, Episodes: eps(1, 50)}},

	// Not episodes
	{"Movies/Heat (1995)/Heat (1995).mkv", Episode{}},
	{"random.mkv", Episode{}},
	{"Show/Show 2019.mkv", Episode{}},
	{"", Episode{}},
}

func TestParseEpisode(t *testing.T) {
	for _, tc := range episodeCases {
		got, ok := ParseEpisode(tc.path)
		wantOK := len(tc.want.Episodes) > 0 || tc.want.AirDate != ""
		if ok != wantOK {
			t.Errorf("ParseEpisode(%q) ok = %v, want %v (got %+v)", tc.path, ok, wantOK, got)
			continue
		}
		if !ok {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseEpisode(%q)\n got  %+v\n want %+v", tc.path, got, tc.want)
		}
	}
}

func TestParseSeasonFolder(t *testing.T) {
	cases := []struct {
		name   string
		season int
		ok     bool
	}{
		{"Season 1", 1, true},
		{"SEASON 1", 1, true},
		{"season 01", 1, true},
		{"Season 10", 10, true},
		{"Season 100", 100, true},
		{"Season1", 1, true},
		{"Season.02", 2, true},
		{"Season_03", 3, true},
		{"Season 2024", 2024, true},
		{"Season 0", 0, true},
		{"Saison 1", 1, true},
		{"Saison 02", 2, true},
		{"Staffel 4", 4, true},
		{"STAFFEL 1", 1, true},
		{"Stagione 1", 1, true},
		{"Temporada 3", 3, true},
		{"sæson 1", 1, true},
		{"Säsong 1", 1, true},
		{"Seizoen 1", 1, true},
		{"Sezon 1", 1, true},
		{"SEZÓNA 1", 1, true},
		{"Sezonul 1", 1, true},
		{"Kausi 1", 1, true},
		{"Series 2", 2, true},
		{"Сезон 1", 1, true},
		{"시즌 1", 1, true},
		{"シーズン 1", 1, true},
		{"S03", 3, true},
		{"s1", 1, true},
		{"S00", 0, true},
		{"Specials", 0, true},
		{"Special", 0, true},
		{"specials", 0, true},
		{"Spéciaux", 0, true},
		{"2", 2, true},
		{"Season 1 (2008)", 1, true},
		{"Season 01 [1080p]", 1, true},
		{"Season 1 - The Beginning", 1, true},
		{"/Drive/Seinfeld/S02", 2, true},
		{`C:\TV\Seinfeld\Season 2`, 2, true},
		{"The.Wonder.Years.S04.PDTV.x264-JCH", 4, true},
		{"Breaking.Bad.Season.2.1080p.BluRay.x264", 2, true},
		{"Breaking Bad S02", 2, true},
		{"Seinfeld Season 2", 0, false},
		{"Season of the Witch", 0, false},
		{"Seasons", 0, false},
		{"Season", 0, false},
		{"Breaking Bad", 0, false},
		{"Show S01E01", 0, false},
		{"Spaced", 0, false},
		{"2009", 0, false},
		{"Extras", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		s, ok := ParseSeasonFolder(tc.name)
		if ok != tc.ok || (ok && s != tc.season) {
			t.Errorf("ParseSeasonFolder(%q) = %d, %v; want %d, %v", tc.name, s, ok, tc.season, tc.ok)
		}
	}
}

func TestParseSeriesFolder(t *testing.T) {
	cases := []struct {
		name string
		want Series
	}{
		{"Breaking Bad (2008) {tvdb-81189}", Series{Title: "Breaking Bad", Year: 2008, IDs: ProviderIDs{TVDB: "81189"}}},
		{"Breaking Bad (2008) [tvdbid-81189]", Series{Title: "Breaking Bad", Year: 2008, IDs: ProviderIDs{TVDB: "81189"}}},
		{"Breaking Bad", Series{Title: "Breaking Bad"}},
		{"The Office (US)", Series{Title: "The Office (US)"}},
		{"Doctor Who (2005)", Series{Title: "Doctor Who", Year: 2005}},
		{"Case Closed (1996-2007)", Series{Title: "Case Closed", Year: 1996}},
		{"Friends (1994-2004) {imdb-tt0108778}", Series{Title: "Friends", Year: 1994, IDs: ProviderIDs{IMDB: "tt0108778"}}},
		{"The.Wire.2002.1080p.BluRay", Series{Title: "The Wire", Year: 2002}},
		{"Le Bureau des légendes (2015)", Series{Title: "Le Bureau des légendes", Year: 2015}},
		{"24", Series{Title: "24"}},
		{"1883 (2021)", Series{Title: "1883", Year: 2021}},
		{"/media/TV/Dark (2017) [tmdbid-70523]", Series{Title: "Dark", Year: 2017, IDs: ProviderIDs{TMDB: "70523"}}},
		{"Marvel's Agents of S.H.I.E.L.D. (2013)", Series{Title: "Marvel's Agents of S.H.I.E.L.D.", Year: 2013}},
		{"[Group] Some Anime", Series{Title: "Some Anime"}},
		{"Lupin (2021) - MULTi 1080p", Series{Title: "Lupin", Year: 2021}},
	}
	for _, tc := range cases {
		if got := ParseSeriesFolder(tc.name); got != tc.want {
			t.Errorf("ParseSeriesFolder(%q) = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func FuzzParseEpisode(f *testing.F) {
	for _, tc := range episodeCases {
		f.Add(tc.path)
	}
	f.Fuzz(func(t *testing.T, path string) {
		ep, ok := ParseEpisode(path)
		if !ok {
			return
		}
		if len(ep.Episodes) == 0 && ep.AirDate == "" {
			t.Fatalf("ok without numbering: %+v", ep)
		}
		if len(ep.Episodes) > maxEpisodes+maxRangeWidth {
			t.Fatalf("too many episodes: %d", len(ep.Episodes))
		}
		for i, n := range ep.Episodes {
			if n < 0 || (i > 0 && n <= ep.Episodes[i-1]) {
				t.Fatalf("bad episode list %v", ep.Episodes)
			}
		}
		if ep.Season < -1 {
			t.Fatalf("bad season %d", ep.Season)
		}
		if ep.Absolute < 0 || ep.Year != 0 && (ep.Year < 1900 || ep.Year > 2100) {
			t.Fatalf("bad absolute/year %+v", ep)
		}
		if ep.AirDate != "" && len(ep.AirDate) != 10 {
			t.Fatalf("bad air date %q", ep.AirDate)
		}
		if ep.Title != trimSpace(ep.Title) || ep.SeriesTitle != trimSpace(ep.SeriesTitle) {
			t.Fatalf("untrimmed titles %+v", ep)
		}
	})
}
