package tmdb

// TMDB v3 response shapes (only the fields we map).

type named struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type movieHit struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	ReleaseDate   string  `json:"release_date"`
	Overview      string  `json:"overview"`
	PosterPath    string  `json:"poster_path"`
	Popularity    float64 `json:"popularity"`
}

type tvHit struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	OriginalName string  `json:"original_name"`
	FirstAirDate string  `json:"first_air_date"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	Popularity   float64 `json:"popularity"`
}

type movieSearch struct {
	Results []movieHit `json:"results"`
}

type tvSearch struct {
	Results []tvHit `json:"results"`
}

type findResp struct {
	MovieResults []movieHit `json:"movie_results"`
	TVResults    []tvHit    `json:"tv_results"`
}

type castMember struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	Order       int    `json:"order"`
	ProfilePath string `json:"profile_path"`
	// aggregate_credits only
	Roles []struct {
		Character    string `json:"character"`
		EpisodeCount int    `json:"episode_count"`
	} `json:"roles"`
}

type crewMember struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Job         string `json:"job"`
	Department  string `json:"department"`
	ProfilePath string `json:"profile_path"`
}

type credits struct {
	Cast []castMember `json:"cast"`
	Crew []crewMember `json:"crew"`
}

type image struct {
	FilePath    string  `json:"file_path"`
	Language    *string `json:"iso_639_1"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	VoteAverage float64 `json:"vote_average"`
	VoteCount   int     `json:"vote_count"`
}

type images struct {
	Posters   []image `json:"posters"`
	Backdrops []image `json:"backdrops"`
	Logos     []image `json:"logos"`
}

type externalIDs struct {
	ImdbID string `json:"imdb_id"`
	TvdbID *int64 `json:"tvdb_id"`
}

type movieDetails struct {
	ID                  int     `json:"id"`
	Title               string  `json:"title"`
	OriginalTitle       string  `json:"original_title"`
	Overview            string  `json:"overview"`
	Tagline             string  `json:"tagline"`
	ReleaseDate         string  `json:"release_date"`
	Runtime             int     `json:"runtime"`
	VoteAverage         float64 `json:"vote_average"`
	ImdbID              string  `json:"imdb_id"`
	PosterPath          string  `json:"poster_path"`
	BackdropPath        string  `json:"backdrop_path"`
	Genres              []named `json:"genres"`
	ProductionCompanies []named `json:"production_companies"`
	BelongsToCollection *named  `json:"belongs_to_collection"`
	Credits             credits `json:"credits"`
	Images              images  `json:"images"`
	ReleaseDates        struct {
		Results []struct {
			Country      string `json:"iso_3166_1"`
			ReleaseDates []struct {
				Certification string `json:"certification"`
				Type          int    `json:"type"`
				ReleaseDate   string `json:"release_date"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
	ExternalIDs externalIDs `json:"external_ids"`
}

type tvDetails struct {
	ID                  int     `json:"id"`
	Name                string  `json:"name"`
	OriginalName        string  `json:"original_name"`
	Overview            string  `json:"overview"`
	Tagline             string  `json:"tagline"`
	FirstAirDate        string  `json:"first_air_date"`
	LastAirDate         string  `json:"last_air_date"`
	Status              string  `json:"status"`
	EpisodeRunTime      []int   `json:"episode_run_time"`
	VoteAverage         float64 `json:"vote_average"`
	PosterPath          string  `json:"poster_path"`
	BackdropPath        string  `json:"backdrop_path"`
	Genres              []named `json:"genres"`
	Networks            []named `json:"networks"`
	ProductionCompanies []named `json:"production_companies"`
	CreatedBy           []struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		ProfilePath string `json:"profile_path"`
	} `json:"created_by"`
	AggregateCredits credits `json:"aggregate_credits"`
	Images           images  `json:"images"`
	ContentRatings   struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Rating  string `json:"rating"`
		} `json:"results"`
	} `json:"content_ratings"`
	ExternalIDs externalIDs `json:"external_ids"`
}

type tvEpisode struct {
	ID            int          `json:"id"`
	Name          string       `json:"name"`
	Overview      string       `json:"overview"`
	AirDate       string       `json:"air_date"`
	EpisodeNumber int          `json:"episode_number"`
	SeasonNumber  int          `json:"season_number"`
	Runtime       int          `json:"runtime"`
	VoteAverage   float64      `json:"vote_average"`
	StillPath     string       `json:"still_path"`
	GuestStars    []castMember `json:"guest_stars"`
	Crew          []crewMember `json:"crew"`
}

type tvSeason struct {
	ID           int         `json:"id"`
	Name         string      `json:"name"`
	Overview     string      `json:"overview"`
	AirDate      string      `json:"air_date"`
	PosterPath   string      `json:"poster_path"`
	SeasonNumber int         `json:"season_number"`
	VoteAverage  float64     `json:"vote_average"`
	Episodes     []tvEpisode `json:"episodes"`
	Images       images      `json:"images"`
}
