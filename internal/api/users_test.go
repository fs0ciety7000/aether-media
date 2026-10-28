package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func send(t *testing.T, c *http.Client, method, url, body string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

// member creates a non-admin user and returns a client logged in as them.
func member(t *testing.T, ts *httptest.Server, admin *http.Client, body string) *http.Client {
	t.Helper()
	if code, b := send(t, admin, "POST", ts.URL+"/api/v1/users", body); code != 201 {
		t.Fatalf("create user: %d %s", code, b)
	}
	var u struct{ Name string }
	_ = json.Unmarshal([]byte(body), &u)
	code, b := send(t, http.DefaultClient, "POST", ts.URL+"/api/v1/auth/login", `{"name":"`+u.Name+`","password":"motdepasse"}`)
	if code != 200 {
		t.Fatalf("login: %d %s", code, b)
	}
	var out struct{ Token string }
	_ = json.Unmarshal(b, &out)
	return &http.Client{Transport: bearerTransport{out.Token}}
}

func TestAuthRequiredAndRoles(t *testing.T) {
	ts := browseServer(t)
	admin := authed
	if code, _ := send(t, http.DefaultClient, "GET", ts.URL+"/api/v1/items", ""); code != 401 {
		t.Errorf("anonymous items → %d", code)
	}
	if code, _ := send(t, http.DefaultClient, "POST", ts.URL+"/api/v1/setup", `{"name":"x","password":"motdepasse"}`); code != 409 {
		t.Errorf("second setup → %d", code)
	}
	bob := member(t, ts, admin, `{"name":"bob","password":"motdepasse"}`)
	if code, _ := send(t, bob, "GET", ts.URL+"/api/v1/items", ""); code != 200 {
		t.Errorf("member items → %d", code)
	}
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/v1/libraries"}, {"GET", "/api/v1/users"}, {"PATCH", "/api/v1/items/1"},
		{"POST", "/api/v1/libraries/1/scan"}, {"GET", "/api/v1/jobs"},
	} {
		if code, _ := send(t, bob, c.method, ts.URL+c.path, `{}`); code != 403 {
			t.Errorf("member %s %s → %d", c.method, c.path, code)
		}
	}
	// Members never see file paths or sources.
	_, b := send(t, bob, "GET", ts.URL+"/api/v1/items", "")
	if strings.Contains(string(b), `"path"`) {
		t.Errorf("member sees paths: %s", b)
	}
	_, b = send(t, bob, "GET", ts.URL+"/api/v1/libraries", "")
	if strings.Contains(string(b), `"root"`) {
		t.Errorf("member sees sources: %s", b)
	}
	// Last admin is protected.
	if code, _ := send(t, admin, "DELETE", ts.URL+"/api/v1/users/1", ""); code != 409 {
		t.Errorf("delete last admin → %d", code)
	}
	// Logout revokes the token.
	send(t, bob, "POST", ts.URL+"/api/v1/auth/logout", "")
	if code, _ := send(t, bob, "GET", ts.URL+"/api/v1/me", ""); code != 401 {
		t.Errorf("after logout → %d", code)
	}
}

func TestLoginThrottleAndCSRF(t *testing.T) {
	ts := browseServer(t)
	for range 10 {
		send(t, http.DefaultClient, "POST", ts.URL+"/api/v1/auth/login", `{"name":"admin","password":"mauvais"}`)
	}
	if code, _ := send(t, http.DefaultClient, "POST", ts.URL+"/api/v1/auth/login", `{"name":"admin","password":"motdepasse"}`); code != 429 {
		t.Errorf("throttled login → %d", code)
	}
	// Cookie-authenticated write from another origin is refused.
	code, b := send(t, authed, "POST", ts.URL+"/api/v1/collections", `{"name":"x"}`)
	if code != 201 {
		t.Fatalf("bearer write → %d %s", code, b)
	}
	tok := authed.Transport.(bearerTransport).token
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/collections", strings.NewReader(`{"name":"y"}`))
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	req.Header.Set("Origin", "https://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Errorf("cross-origin cookie write → %d", res.StatusCode)
	}
}

func TestLibraryAccessAndParental(t *testing.T) {
	ts := browseServer(t)
	// Heat gets a 16+ rating, Ratatouille "TP".
	var page itemPage
	getJSON(t, ts, "/api/v1/items", &page)
	byTitle := map[string]int64{}
	for _, it := range page.Items {
		byTitle[it.Title] = it.ID
	}
	send(t, authed, "PATCH", ts.URL+"/api/v1/items/"+itoa(byTitle["Heat"]), `{"officialRating":"-16"}`)
	send(t, authed, "PATCH", ts.URL+"/api/v1/items/"+itoa(byTitle["Ratatouille"]), `{"officialRating":"TP"}`)

	kid := member(t, ts, authed, `{"name":"kid","password":"motdepasse","maxRatingAge":12}`)
	_, b := send(t, kid, "GET", ts.URL+"/api/v1/items", "")
	if strings.Contains(string(b), `"Heat"`) || !strings.Contains(string(b), "Ratatouille") {
		t.Errorf("parental filter: %s", b)
	}
	if code, _ := send(t, kid, "GET", ts.URL+"/api/v1/items/"+itoa(byTitle["Heat"]), ""); code != 404 {
		t.Errorf("hidden detail → %d", code)
	}
	strict := member(t, ts, authed, `{"name":"strict","password":"motdepasse","maxRatingAge":18,"blockUnrated":true}`)
	_, b = send(t, strict, "GET", ts.URL+"/api/v1/items", "")
	var p itemPage
	_ = json.Unmarshal(b, &p)
	if p.Total != 2 { // only the two rated films
		t.Errorf("blockUnrated total = %d (%s)", p.Total, b)
	}
	none := member(t, ts, authed, `{"name":"guest","password":"motdepasse","allLibraries":false,"libraryIds":[]}`)
	_, b = send(t, none, "GET", ts.URL+"/api/v1/items", "")
	_ = json.Unmarshal(b, &p)
	if p.Total != 0 {
		t.Errorf("no library access total = %d", p.Total)
	}
	if code, _ := send(t, none, "GET", ts.URL+"/api/v1/libraries/1", ""); code != 404 {
		t.Errorf("forbidden library → %d", code)
	}
}

func TestUserDataAndCollections(t *testing.T) {
	ts := browseServer(t)
	var page itemPage
	getJSON(t, ts, "/api/v1/items?sort=title", &page)
	id := page.Items[0].ID
	code, b := send(t, authed, "PUT", ts.URL+"/api/v1/items/"+itoa(id)+"/userdata", `{"favorite":true,"positionMs":60000}`)
	if code != 200 || !strings.Contains(string(b), `"favorite":true`) || !strings.Contains(string(b), `"positionMs":60000`) {
		t.Fatalf("userdata: %d %s", code, b)
	}
	getJSON(t, ts, "/api/v1/items?favorite=true", &page)
	if page.Total != 1 {
		t.Errorf("favorites = %d", page.Total)
	}
	getJSON(t, ts, "/api/v1/items?inProgress=true", &page)
	if page.Total != 1 {
		t.Errorf("in progress = %d", page.Total)
	}
	send(t, authed, "PUT", ts.URL+"/api/v1/items/"+itoa(id)+"/userdata", `{"played":true}`)
	getJSON(t, ts, "/api/v1/items?played=false", &page)
	if page.Total != 3 {
		t.Errorf("unplayed = %d", page.Total)
	}
	// Another user has independent data.
	bob := member(t, ts, authed, `{"name":"bob","password":"motdepasse"}`)
	_, b = send(t, bob, "GET", ts.URL+"/api/v1/items?played=true", "")
	var p itemPage
	_ = json.Unmarshal(b, &p)
	if p.Total != 0 {
		t.Errorf("bob sees admin's played state")
	}

	code, b = send(t, authed, "POST", ts.URL+"/api/v1/collections", `{"name":"Soirée"}`)
	var c struct{ ID string }
	_ = json.Unmarshal(b, &c)
	if code != 201 || c.ID == "" {
		t.Fatalf("create collection: %d %s", code, b)
	}
	if code, _ := send(t, authed, "PUT", ts.URL+"/api/v1/collections/"+c.ID+"/items/"+itoa(id), ""); code != 204 {
		t.Errorf("add to collection → %d", code)
	}
	_, b = send(t, authed, "GET", ts.URL+"/api/v1/collections/"+c.ID+"/items", "")
	if !strings.Contains(string(b), `"count":1`) {
		t.Errorf("collection items: %s", b)
	}
	// Private collection: invisible to bob; sharing needs admin.
	if code, _ := send(t, bob, "GET", ts.URL+"/api/v1/collections/"+c.ID+"/items", ""); code != 404 {
		t.Errorf("bob reads private collection → %d", code)
	}
	if code, _ := send(t, bob, "POST", ts.URL+"/api/v1/collections", `{"name":"x","shared":true}`); code != 403 {
		t.Errorf("member shared collection → %d", code)
	}
	_ = strconv.Itoa
}

func TestWatchedEnough(t *testing.T) {
	minute := int64(60_000)
	cases := []struct {
		kind     string
		pos, dur int64
		want     bool
	}{
		{"movie", 90 * minute, 100 * minute, true},
		{"movie", 50 * minute, 100 * minute, false},
		{"movie", 119 * minute, 121 * minute, true}, // credits
		{"episode", 41*minute + 40_000, 42 * minute, true},
		{"episode", 30 * minute, 42 * minute, false},
		{"movie", 0, 100 * minute, false},
	}
	for _, c := range cases {
		if got := watchedEnough(c.kind, c.pos, c.dur); got != c.want {
			t.Errorf("%+v → %v", c, got)
		}
	}
}

func TestNextUp(t *testing.T) {
	ts := showsServer(t, []string{
		"Lupin (2021)/Saison 1/Lupin.S01E01.mkv", "Lupin (2021)/Saison 1/Lupin.S01E02.mkv",
		"Lupin (2021)/Saison 1/Lupin.S01E03.mkv", "Lupin (2021)/Saison 2/Lupin.S02E01.mkv",
		"Lupin (2021)/Specials/Lupin.S00E01.mkv",
	})
	var eps itemPage
	getJSON(t, ts, "/api/v1/items?kind=episode&limit=50", &eps)
	ep := map[string]int64{}
	for _, it := range eps.Items {
		ep[strconv.FormatInt(*it.ParentIndexNumber, 10)+"x"+strconv.FormatInt(*it.IndexNumber, 10)] = it.ID
	}
	var home homeDTO
	getJSON(t, ts, "/api/v1/home", &home)
	if len(home.NextUp) != 0 {
		t.Fatalf("nothing watched yet, nextUp = %v", titles(home.NextUp))
	}
	send(t, authed, "PUT", ts.URL+"/api/v1/items/"+itoa(ep["1x1"])+"/userdata", `{"played":true}`)
	getJSON(t, ts, "/api/v1/home", &home)
	if len(home.NextUp) != 1 || home.NextUp[0].ID != ep["1x2"] {
		t.Fatalf("after S1E1 nextUp = %+v", titles(home.NextUp))
	}
	send(t, authed, "PUT", ts.URL+"/api/v1/items/"+itoa(ep["1x3"])+"/userdata", `{"played":true}`)
	getJSON(t, ts, "/api/v1/home", &home)
	if len(home.NextUp) != 1 || home.NextUp[0].ID != ep["2x1"] {
		t.Errorf("after S1E3 nextUp should be S2E1, got %+v", titles(home.NextUp))
	}
	// Series shows its unplayed count.
	var series itemPage
	getJSON(t, ts, "/api/v1/items?kind=series", &series)
	if series.Items[0].UnplayedCount == nil || *series.Items[0].UnplayedCount != 3 {
		t.Errorf("unplayed count = %v", series.Items[0].UnplayedCount)
	}
	// Marking the series watched marks every episode.
	send(t, authed, "PUT", ts.URL+"/api/v1/items/"+itoa(series.Items[0].ID)+"/userdata", `{"played":true}`)
	getJSON(t, ts, "/api/v1/items?kind=episode&played=false", &eps)
	if eps.Total != 0 {
		t.Errorf("unplayed after series watched = %d", eps.Total)
	}
}
