package naming

import "testing"

var movieCases = []struct {
	path string
	want Movie
}{
	// Plain "Title (Year)"
	{"Heat (1995).mkv", Movie{Title: "Heat", Year: 1995}},
	{"Movies/Heat (1995)/heat.mkv", Movie{Title: "Heat", Year: 1995}},
	{"Movies/Heat (1995)/Heat (1995).mkv", Movie{Title: "Heat", Year: 1995}},
	{`C:\Movies\Heat (1995)\heat.mkv`, Movie{Title: "Heat", Year: 1995}},
	{`d:\movies\300 (2006).mkv`, Movie{Title: "300", Year: 2006}},
	{"d:/movies/300 (2006).mkv", Movie{Title: "300", Year: 2006}},
	{"/server/Movies/300 (2007)/300 (2006).mkv", Movie{Title: "300", Year: 2006}},
	{"300 2 (2006).mkv", Movie{Title: "300 2", Year: 2006}},
	{"300 - 2 (2006).mkv", Movie{Title: "300 - 2", Year: 2006}},
	{"300 2001 (2006).mkv", Movie{Title: "300 2001", Year: 2006}},
	{"The Wolf of Wall Street (2013).mkv", Movie{Title: "The Wolf of Wall Street", Year: 2013}},
	{"The Wolf of Wall Street 2 (2013).mkv", Movie{Title: "The Wolf of Wall Street 2", Year: 2013}},
	{"The Wolf of Wall Street - 2 (2013).mkv", Movie{Title: "The Wolf of Wall Street - 2", Year: 2013}},
	{"The Wolf of Wall Street 2001 (2013).mkv", Movie{Title: "The Wolf of Wall Street 2001", Year: 2013}},
	{"St. Vincent (2014).mkv", Movie{Title: "St. Vincent", Year: 2014}},
	{"Super movie(2009).mp4", Movie{Title: "Super movie", Year: 2009}},
	{"Run lola run (lola rennt) (2009).mp4", Movie{Title: "Run lola run (lola rennt)", Year: 2009}},
	{"My Movie (1997) - GreatestReleaseGroup 2019.mp4", Movie{Title: "My Movie", Year: 1997}},
	{"First Man (2018) 1080p.mkv", Movie{Title: "First Man", Year: 2018}},
	{"Robin Hood [Multi-Subs] [2018].mkv", Movie{Title: "Robin Hood", Year: 2018}},
	{"[1985] Opportunities (Let's make lots of money) (1985).mkv", Movie{Title: "Opportunities (Let's make lots of money)", Year: 1985}},
	{"(1995) Heat.mkv", Movie{Title: "Heat", Year: 1995}},
	{"American.Psycho.mkv", Movie{Title: "American Psycho"}},
	{"American Psycho.mkv", Movie{Title: "American Psycho"}},
	{"[rec].mkv", Movie{Title: "rec"}},
	{"Hello.mkv", Movie{Title: "Hello"}},
	{"Movies/Hello.mkv", Movie{Title: "Hello"}},
	{"Last.Call.for.Nowhere.WEB-DL.1080p", Movie{Title: "Last Call for Nowhere"}},

	// Titles that are, or contain, years
	{"1917 (2019).mkv", Movie{Title: "1917", Year: 2019}},
	{"1917.2019.1080p.WEB-DL.DDP5.1.H.264-GRP.mkv", Movie{Title: "1917", Year: 2019}},
	{"1917.mkv", Movie{Title: "1917"}},
	{"2001 A Space Odyssey (1968).mkv", Movie{Title: "2001 A Space Odyssey", Year: 1968}},
	{"2001.A.Space.Odyssey.1968.1080p.BluRay.x264-GRP.mkv", Movie{Title: "2001 A Space Odyssey", Year: 1968}},
	{"Movies/2001 A Space Odyssey (1968)/2001.A.Space.Odyssey.mkv", Movie{Title: "2001 A Space Odyssey", Year: 1968}},
	{"Blade Runner 2049 (2017).mkv", Movie{Title: "Blade Runner 2049", Year: 2017}},
	{"Blade.Runner.2049.2017.2160p.UHD.BluRay.REMUX.HDR.HEVC.Atmos-EPSiLON.mkv", Movie{Title: "Blade Runner 2049", Year: 2017}},
	{"Wonder.Woman.1984.2020.1080p.mkv", Movie{Title: "Wonder Woman 1984", Year: 2020}},
	{"Wonder Woman 1984 (2020).mkv", Movie{Title: "Wonder Woman 1984", Year: 2020}},
	{"2012 (2009).mkv", Movie{Title: "2012", Year: 2009}},
	{"2012.2009.1080p.BluRay.mkv", Movie{Title: "2012", Year: 2009}},
	{"Drug War 2013.mp4", Movie{Title: "Drug War", Year: 2013}},
	{"First Man 2018 1080p.mkv", Movie{Title: "First Man", Year: 2018}},
	{"Movie.2010.mkv", Movie{Title: "Movie", Year: 2010}},
	{"Se7en (1995).mkv", Movie{Title: "Se7en", Year: 1995}},
	{"Blade Runner (1982).mkv", Movie{Title: "Blade Runner", Year: 1982}},

	// Scene releases
	{"The.Wolf.of.Wall.Street.2013.1080p.BluRay.x264.mkv", Movie{Title: "The Wolf of Wall Street", Year: 2013}},
	{"curse.of.chucky.2013.stv.unrated.multi.1080p.bluray.x264-rough.mkv", Movie{Title: "curse of chucky", Year: 2013, Edition: "unrated"}},
	{"Arrival.2016.2160p.Blu-Ray.HEVC.mkv", Movie{Title: "Arrival", Year: 2016}},
	{"Maximum Ride - 2016 - WEBDL-1080p - x264 AC3.mkv", Movie{Title: "Maximum Ride", Year: 2016}},
	{"3.Days.to.Kill.2014.720p.BluRay.x264.YIFY.mkv", Movie{Title: "3 Days to Kill", Year: 2014}},
	{"3 days to kill (2005).mkv", Movie{Title: "3 days to kill", Year: 2005}},
	{"Rain Man 1988 REMASTERED 1080p BluRay x264 AAC - Ozlem.mp4", Movie{Title: "Rain Man", Year: 1988, Edition: "REMASTERED"}},
	{"Crouching.Tiger.Hidden.Dragon.4K.UltraHD.HDR.BDrip-HDC.mkv", Movie{Title: "Crouching Tiger Hidden Dragon"}},
	{"Crouching.Tiger.Hidden.Dragon.BDrip.mkv", Movie{Title: "Crouching Tiger Hidden Dragon"}},
	{"Movie 1080p.mkv", Movie{Title: "Movie"}},
	{"The.Matrix.1999.mkv", Movie{Title: "The Matrix", Year: 1999}},
	{"Movies/The Matrix (1999)/the.matrix.1999.1080p.mkv", Movie{Title: "The Matrix", Year: 1999}},
	{"The.Movie.Title.2010.1080p.BluRay.x264.DTS-HD.MA.5.1-GRP.mkv", Movie{Title: "The Movie Title", Year: 2010}},
	{"Movie.Title.2010.PROPER.1080p.mkv", Movie{Title: "Movie Title", Year: 2010}},
	{"Movie Title 2010 REPACK 720p HDTV x264-GRP.mkv", Movie{Title: "Movie Title", Year: 2010}},
	{"Spider-Man.No.Way.Home.2021.2160p.WEB-DL.x265.10bit.HDR.DDP5.1.Atmos-GRP.mkv", Movie{Title: "Spider-Man No Way Home", Year: 2021}},
	{"Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX.mkv", Movie{Title: "Dune Part Two", Year: 2024}},
	{"Mission.Impossible.Fallout.2018.IMAX.1080p.mkv", Movie{Title: "Mission Impossible Fallout", Year: 2018, Edition: "IMAX"}},
	{"Oppenheimer.2023.1080p.AMZN.WEB-DL.DDP5.1.H.264-FLUX.mkv", Movie{Title: "Oppenheimer", Year: 2023}},
	{"Interstellar.2014.2160p.UHD.BluRay.x265.10bit.HDR.TrueHD.7.1.Atmos-DON.mkv", Movie{Title: "Interstellar", Year: 2014}},
	{"Parasite.2019.KOREAN.1080p.BluRay.x264.DTS-FGT.mkv", Movie{Title: "Parasite", Year: 2019}},
	{"Joker.2019.HDR.2160p.WEB.H265-DEFLATE.mkv", Movie{Title: "Joker", Year: 2019}},
	{"Up.2009.720p.BrRip.x264.YIFY.mp4", Movie{Title: "Up", Year: 2009}},
	{"Heat.1995.1080p.BluRay.x264-GRP.mkv", Movie{Title: "Heat", Year: 1995}},
	{"Tenet.2020.IMAX.HDR.2160p.WEB.h265-NAISU.mkv", Movie{Title: "Tenet", Year: 2020, Edition: "IMAX"}},

	// Kodi / Radarr naming
	{"Movie Name (2010) Bluray-1080p.mkv", Movie{Title: "Movie Name", Year: 2010}},
	{"The Movie Title (2010) - Bluray-1080p Proper.mkv", Movie{Title: "The Movie Title", Year: 2010}},
	{"Movie Name (2010) [1080p] [BluRay] [5.1] [YTS.MX].mp4", Movie{Title: "Movie Name", Year: 2010}},
	{"[YTS] Movie Name (2015) [1080p].mp4", Movie{Title: "Movie Name", Year: 2015}},
	{"Movie Name (2015) (1080p BluRay x265 HEVC 10bit AAC 5.1 Tigole).mkv", Movie{Title: "Movie Name", Year: 2015}},
	{"The Movie (2010)/The Movie (2010) - 1080p.mkv", Movie{Title: "The Movie", Year: 2010}},
	{"Movie Name (2010) {tmdb-12345}.mkv", Movie{Title: "Movie Name", Year: 2010, IDs: ProviderIDs{TMDB: "12345"}}},
	{"The Matrix (1999) [imdbid-tt0133093].mkv", Movie{Title: "The Matrix", Year: 1999, IDs: ProviderIDs{IMDB: "tt0133093"}}},
	{"The Matrix (1999) {imdb-tt0133093}.mkv", Movie{Title: "The Matrix", Year: 1999, IDs: ProviderIDs{IMDB: "tt0133093"}}},
	{"Movies/The Matrix (1999) [tmdbid-603]/The.Matrix.1999.1080p.mkv", Movie{Title: "The Matrix", Year: 1999, IDs: ProviderIDs{TMDB: "603"}}},
	{"Inception (2010) [tmdbid=27205].mkv", Movie{Title: "Inception", Year: 2010, IDs: ProviderIDs{TMDB: "27205"}}},
	{"Inception (2010) {tmdb-27205} {imdb-tt1375666}.mkv", Movie{Title: "Inception", Year: 2010, IDs: ProviderIDs{TMDB: "27205", IMDB: "tt1375666"}}},
	{"Movies/Alien (1979) {imdb-tt0078748}/Alien (1979) {edition-Director's Cut}.mkv", Movie{Title: "Alien", Year: 1979, Edition: "Director's Cut", IDs: ProviderIDs{IMDB: "tt0078748"}}},
	{"Movies/Inception (2010) {tmdb-27205}/movie.mkv", Movie{Title: "Inception", Year: 2010, IDs: ProviderIDs{TMDB: "27205"}}},
	{"480 Super movie [tmdbid=12345].mp4", Movie{Title: "480 Super movie", IDs: ProviderIDs{TMDB: "12345"}}},
	{"Movie (2010) [tmdbid-abc].mkv", Movie{Title: "Movie", Year: 2010}},

	// Editions
	{"Blade Runner (1982) {edition-Final Cut}.mkv", Movie{Title: "Blade Runner", Year: 1982, Edition: "Final Cut"}},
	{"Blade.Runner.1982.Final.Cut.1080p.BluRay.x264.mkv", Movie{Title: "Blade Runner", Year: 1982, Edition: "Final Cut"}},
	{"Aliens (1986) {edition-Director's Cut} [imdbid-tt0090605].mkv", Movie{Title: "Aliens", Year: 1986, Edition: "Director's Cut", IDs: ProviderIDs{IMDB: "tt0090605"}}},
	{"Aliens.Special.Edition.1986.1080p.mkv", Movie{Title: "Aliens", Year: 1986, Edition: "Special Edition"}},
	{"The.Lord.of.the.Rings.The.Fellowship.of.the.Ring.2001.EXTENDED.1080p.BluRay.x264.mkv", Movie{Title: "The Lord of the Rings The Fellowship of the Ring", Year: 2001, Edition: "EXTENDED"}},
	{"Apocalypse Now Redux (1979).mkv", Movie{Title: "Apocalypse Now Redux", Year: 1979}},
	{"Apocalypse.Now.1979.Redux.1080p.mkv", Movie{Title: "Apocalypse Now", Year: 1979, Edition: "Redux"}},
	{"Uncut Gems (2019).mkv", Movie{Title: "Uncut Gems", Year: 2019}},
	{"The Final Cut (2004).mkv", Movie{Title: "The Final Cut", Year: 2004}},
	{"Movies/Some Movie (2010)/Some Movie (2010) - Director's Cut.mkv", Movie{Title: "Some Movie", Year: 2010, Edition: "Director's Cut"}},
	{"Some Movie (2010) - [Director's Cut].mkv", Movie{Title: "Some Movie", Year: 2010, Edition: "Director's Cut"}},
	{"Some Movie (2010) Extended Edition.mkv", Movie{Title: "Some Movie", Year: 2010, Edition: "Extended Edition"}},
	{"The Lord of the Rings Extended Edition.mkv", Movie{Title: "The Lord of the Rings", Edition: "Extended Edition"}},
	{"Le.Seigneur.des.Anneaux.La.Communaute.de.l'Anneau.2001.VERSION.LONGUE.MULTi.1080p.mkv", Movie{Title: "Le Seigneur des Anneaux La Communaute de l'Anneau", Year: 2001, Edition: "VERSION LONGUE"}},
	{"Brazil.1985.Directors.Cut.720p.mkv", Movie{Title: "Brazil", Year: 1985, Edition: "Directors Cut"}},
	{"Movie.2010.UNRATED.DVDRip.XviD.avi", Movie{Title: "Movie", Year: 2010, Edition: "UNRATED"}},
	{"Movie (2010) - Theatrical Cut.mkv", Movie{Title: "Movie", Year: 2010, Edition: "Theatrical Cut"}},

	{"Movies/Mad Max Fury Road (2015)/Mad.Max.Fury.Road.2015.Black.and.Chrome.Edition.mkv", Movie{Title: "Mad Max Fury Road", Year: 2015, Edition: "Black and Chrome Edition"}},
	{"The.Dark.Knight.2008.IMAX.2160p.mkv", Movie{Title: "The Dark Knight", Year: 2008, Edition: "IMAX"}},
	{"Movie (2010) [Bluray-1080p][DTS-HD MA 5.1][x264]-GRP.mkv", Movie{Title: "Movie", Year: 2010}},

	// French
	{"Amélie (2001).mkv", Movie{Title: "Amélie", Year: 2001}},
	{"Le Fabuleux Destin d'Amélie Poulain (2001).mkv", Movie{Title: "Le Fabuleux Destin d'Amélie Poulain", Year: 2001}},
	{"Intouchables.2011.FRENCH.1080p.BluRay.x264-LOST.mkv", Movie{Title: "Intouchables", Year: 2011}},
	{"Les.Visiteurs.1993.TRUEFRENCH.DVDRip.XviD-FuN.avi", Movie{Title: "Les Visiteurs", Year: 1993}},
	{"La.Haine.1995.MULTi.VFF.1080p.BluRay.x264-GRP.mkv", Movie{Title: "La Haine", Year: 1995}},
	{"OSS.117.Le.Caire.Nid.D.Espions.2006.FRENCH.720p.BluRay.x264.mkv", Movie{Title: "OSS 117 Le Caire Nid D Espions", Year: 2006}},
	{"Astérix et Obélix - Mission Cléopâtre (2002).mkv", Movie{Title: "Astérix et Obélix - Mission Cléopâtre", Year: 2002}},
	{"Les Misérables (2012).mkv", Movie{Title: "Les Misérables", Year: 2012}},
	{"Mon Film (2015) VOSTFR.mkv", Movie{Title: "Mon Film", Year: 2015}},
	{"Movie.Title.2019.VOSTFR.720p.WEBRip.mkv", Movie{Title: "Movie Title", Year: 2019}},
	{"Les Choristes 2004 FRENCH DVDRip XviD.avi", Movie{Title: "Les Choristes", Year: 2004}},
	{"Bienvenue.chez.les.Ch'tis.2008.FRENCH.BRRip.mkv", Movie{Title: "Bienvenue chez les Ch'tis", Year: 2008}},
	{"Le.Pacte.des.Loups.2001.MULTi.1080p.BluRay.x264-LOST.mkv", Movie{Title: "Le Pacte des Loups", Year: 2001}},
	{"Le Dîner de cons (1998) VFF 1080p.mkv", Movie{Title: "Le Dîner de cons", Year: 1998}},
	{"Films/La Cité de la peur (1994)/la.cite.de.la.peur.1994.vfq.mkv", Movie{Title: "La Cité de la peur", Year: 1994}},
	{"The French Connection (1971).mkv", Movie{Title: "The French Connection", Year: 1971}},
	{"The.French.Connection.1971.FRENCH.1080p.mkv", Movie{Title: "The French Connection", Year: 1971}},
	{"The French Connection.mkv", Movie{Title: "The French Connection"}},

	// Punctuation, acronyms and abbreviations
	{"E.T. the Extra-Terrestrial (1982).mkv", Movie{Title: "E.T. the Extra-Terrestrial", Year: 1982}},
	{"E.T.The.Extra.Terrestrial.1982.1080p.mkv", Movie{Title: "E.T. The Extra Terrestrial", Year: 1982}},
	{"L.A.Confidential.1997.1080p.mkv", Movie{Title: "L.A. Confidential", Year: 1997}},
	{"Mr. Nobody (2009).mkv", Movie{Title: "Mr. Nobody", Year: 2009}},
	{"Mr.Nobody.2009.1080p.mkv", Movie{Title: "Mr. Nobody", Year: 2009}},
	{"Dr.Strangelove.1964.mkv", Movie{Title: "Dr. Strangelove", Year: 1964}},
	{"Kill.Bill.Vol.1.2003.1080p.mkv", Movie{Title: "Kill Bill Vol. 1", Year: 2003}},
	{"No.Country.for.Old.Men.2007.mkv", Movie{Title: "No Country for Old Men", Year: 2007}},
	{"Ocean's Eleven (2001).mkv", Movie{Title: "Ocean's Eleven", Year: 2001}},
	{"Spider-Man (2002).mkv", Movie{Title: "Spider-Man", Year: 2002}},
	{"X-Men (2000).mkv", Movie{Title: "X-Men", Year: 2000}},
	{"WALL-E (2008).mkv", Movie{Title: "WALL-E", Year: 2008}},
	{"Charlotte's Web (2006).mkv", Movie{Title: "Charlotte's Web", Year: 2006}},
	{"Charlottes.Web.mkv", Movie{Title: "Charlottes Web"}},
	{"Star Wars Episode IV - A New Hope (1977).mkv", Movie{Title: "Star Wars Episode IV - A New Hope", Year: 1977}},
	{"Back to the Future Part II (1989).mkv", Movie{Title: "Back to the Future Part II", Year: 1989}},
	{"Dune Part Two (2024).mkv", Movie{Title: "Dune Part Two", Year: 2024}},
	{"Harry Potter and the Deathly Hallows - Part 1 (2010).mkv", Movie{Title: "Harry Potter and the Deathly Hallows - Part 1", Year: 2010}},
	{"Harry.Potter.and.the.Deathly.Hallows.Part.2.2011.1080p.mkv", Movie{Title: "Harry Potter and the Deathly Hallows Part 2", Year: 2011}},
	{"Movie_Name_2010_720p.mkv", Movie{Title: "Movie Name", Year: 2010}},

	// Stacking
	{"Movie (2009) cd1.mkv", Movie{Title: "Movie", Year: 2009, Part: 1}},
	{"Movie (2009) CD2.avi", Movie{Title: "Movie", Year: 2009, Part: 2}},
	{"Bad Boys (2006) part1.mkv", Movie{Title: "Bad Boys", Year: 2006, Part: 1}},
	{"Bad Boys (2006) part 2.mkv", Movie{Title: "Bad Boys", Year: 2006, Part: 2}},
	{"Bad Boys (2006) - Disc 1.mkv", Movie{Title: "Bad Boys", Year: 2006, Part: 1}},
	{"Bad Boys (2006) pt3.mkv", Movie{Title: "Bad Boys", Year: 2006, Part: 3}},
	{"Bad Boys (2006)-dvd2.mkv", Movie{Title: "Bad Boys", Year: 2006, Part: 2}},
	{"Movie.2009.CD1.XviD-GRP.avi", Movie{Title: "Movie", Year: 2009, Part: 1}},
	{"Movie cd2.avi", Movie{Title: "Movie", Part: 2}},
	{"Heat (1995)-a.mkv", Movie{Title: "Heat", Year: 1995, Part: 1}},
	{"Heat (1995)-b.mkv", Movie{Title: "Heat", Year: 1995, Part: 2}},
	{"Heat (1995) - part b.mkv", Movie{Title: "Heat", Year: 1995, Part: 2}},
	{"Movies/Movie (2010)/CD1/movie.avi", Movie{Title: "Movie", Year: 2010, Part: 1}},
	{"Movies/Movie (2010)/Disc 2/movie.avi", Movie{Title: "Movie", Year: 2010, Part: 2}},
	{"Plan B (2010).mkv", Movie{Title: "Plan B", Year: 2010}},

	// Disc structures
	{"Movie (2010)/VIDEO_TS/VTS_01_1.VOB", Movie{Title: "Movie", Year: 2010}},
	{"Movie (2010)/BDMV/STREAM/00001.m2ts", Movie{Title: "Movie", Year: 2010}},

	// 3D
	{"Avatar (2009) 3D HSBS.mkv", Movie{Title: "Avatar", Year: 2009, Is3D: true}},
	{"Avatar.2009.3D.HSBS.1080p.BluRay.x264.mkv", Movie{Title: "Avatar", Year: 2009, Is3D: true}},
	{"Step Up 3D (2010).mkv", Movie{Title: "Step Up 3D", Year: 2010, Is3D: true}},
	{"Gravity (2013) [3D] [H-SBS].mkv", Movie{Title: "Gravity", Year: 2013, Is3D: true}},
	{"Hugo.2011.1080p.3D.HTAB.mkv", Movie{Title: "Hugo", Year: 2011, Is3D: true}},

	// Extras
	{"Movie (2010) - trailer.mkv", Movie{Title: "Movie", Year: 2010, Extra: ExtraTrailer}},
	{"Movies/Heat (1995)/Trailers/Teaser.mkv", Movie{Title: "Heat", Year: 1995, Extra: ExtraTrailer}},
	{"Movies/Heat (1995)/Trailers/Official Trailer 2.mkv", Movie{Title: "Official Trailer 2", Extra: ExtraTrailer}},
	{"Movies/Heat (1995)/trailer.mkv", Movie{Title: "Heat", Year: 1995, Extra: ExtraTrailer}},
	{"Movies/Heat (1995)/heat-trailer.mkv", Movie{Title: "Heat", Year: 1995, Extra: ExtraTrailer}},
	{"Movies/Heat (1995)/Featurettes/Making Of.mkv", Movie{Title: "Making Of", Extra: ExtraFeaturette}},
	{"Heat.1995.1080p.BluRay.x264-GRP.sample.mkv", Movie{Title: "Heat", Year: 1995, Extra: ExtraSample}},
	{"Movies/Heat (1995)/Sample/heat-sample.mkv", Movie{Title: "Heat", Year: 1995, Extra: ExtraSample}},
	{"Movies/Heat (1995) {tmdb-949}/Behind The Scenes/Shooting.mkv", Movie{Title: "Shooting", Extra: ExtraBehindTheScenes, IDs: ProviderIDs{TMDB: "949"}}},
	{"Movies/Heat (1995)/Heat (1995)-deleted.mkv", Movie{Title: "Heat", Year: 1995, Extra: ExtraDeleted}},
}

func TestParseMovie(t *testing.T) {
	for _, tc := range movieCases {
		got := ParseMovie(tc.path)
		if got != tc.want {
			t.Errorf("ParseMovie(%q)\n got  %+v\n want %+v", tc.path, got, tc.want)
		}
	}
}

func FuzzParseMovie(f *testing.F) {
	for _, tc := range movieCases {
		f.Add(tc.path)
	}
	f.Fuzz(func(t *testing.T, path string) {
		m := ParseMovie(path)
		if m.Year != 0 && (m.Year < 1900 || m.Year > 2100) {
			t.Fatalf("year out of range: %d", m.Year)
		}
		if m.Part < 0 || m.Part > 99 {
			t.Fatalf("bad part %d", m.Part)
		}
		if _, ok := validExtras[m.Extra]; !ok {
			t.Fatalf("unknown extra %q", m.Extra)
		}
		if m.Title != trimSpace(m.Title) || m.Edition != trimSpace(m.Edition) {
			t.Fatalf("untrimmed title/edition %q / %q", m.Title, m.Edition)
		}
		if m.IDs.TMDB != "" && !digitsRe.MatchString(m.IDs.TMDB) || m.IDs.IMDB != "" && !imdbRe.MatchString(m.IDs.IMDB) {
			t.Fatalf("bad ids %+v", m.IDs)
		}
	})
}
