#!/usr/bin/env bash
# Builds a small, realistic demo media tree for Aether (films, series, music) with ffmpeg.
# Usage: web/scripts/demo-library.sh <target-dir>
# Creates <target-dir>/{Films,Series,Musique} and a demo aether.toml next to them
# (MusicBrainz disabled so a demo run makes no network calls).
set -euo pipefail

DEST=${1:?usage: demo-library.sh <target-dir>}
command -v ffmpeg >/dev/null || { echo "ffmpeg est requis" >&2; exit 1; }
mkdir -p "$DEST"
DEST=$(cd "$DEST" && pwd)
FILMS="$DEST/Films"; SERIES="$DEST/Series"; MUSIC="$DEST/Musique"
mkdir -p "$FILMS" "$SERIES" "$MUSIC"

FF=(ffmpeg -hide_banner -loglevel error -nostdin -y)

# Font for drawtext (optional: plain gradients without one).
FONT=""
if command -v fc-match >/dev/null; then
  for pat in "Nunito:bold" "Inter:bold" "DejaVu Sans:bold" "sans:bold"; do
    f=$(fc-match -f '%{file}' "$pat" 2>/dev/null || true)
    if [[ -n "$f" && -f "$f" ]]; then FONT="$f"; break; fi
  done
fi
HAS_DRAWTEXT=0
FILTERS=$(ffmpeg -hide_banner -filters 2>/dev/null || true)
if [[ -n "$FONT" && "$FILTERS" == *" drawtext "* ]]; then HAS_DRAWTEXT=1; fi

xml() { sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' <<<"$1"; }
# drawtext-safe text: escape \ : ' %
dt() { printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e "s/'/’/g" -e 's/:/\\:/g' -e 's/%/\\%/g'; }

# art <out.jpg> <w> <h> <colorTop> <colorBottom> <title> <subtitle> [seed]
# Muted sky gradient, a soft sun disc with halo, two layers of hills; title set low-left.
art() {
  local out=$1 w=$2 h=$3 c0=$4 c1=$5 title=$6 sub=$7 seed=${8:-0}
  local sx="(W*(0.58+0.02*$((seed % 7))))" sy="(H*0.42)" sr="(min(W,H)*0.12)"
  local d="hypot(X-$sx,Y-$sy)"
  # sun: disc (sharp-ish edge) + halo; hills: back (lighter) and front (darker) silhouettes.
  local sun="(70*clip(($sr-$d)/($sr*0.06),0,1)+26*exp(-($d*$d)/(2*$sr*$sr*4)))"
  local back="gt(Y,H*(0.66+0.035*sin(X/W*5.1+$seed)+0.02*sin(X/W*13+$seed*2)))"
  local front="gt(Y,H*(0.76+0.03*sin(X/W*3.3+$seed*3)+0.015*sin(X/W*17+$seed)))"
  local base="(%s(X,Y)+%s*$sun)"
  local hills="(1-0.35*$back-0.35*$front)"
  local vf="format=rgb24,geq=r='$(printf "$base" r 1.0)*$hills':g='$(printf "$base" g 0.82)*$hills':b='$(printf "$base" b 0.7)*$hills'"
  if [[ $HAS_DRAWTEXT == 1 && -n "$title" ]]; then
    local fs=$(( w / 10 )) ss=$(( w / 30 )) fit=$(( w * 150 / (100 * ${#title}) ))
    (( fit < fs )) && fs=$fit
    vf+=",drawtext=fontfile='$FONT':text='$(dt "$title")':fontcolor=0xF2F6F5:fontsize=$fs:x=w*0.08:y=h*0.86-text_h-$ss*1.6:shadowcolor=black@0.4:shadowx=0:shadowy=2"
    [[ -n "$sub" ]] && vf+=",drawtext=fontfile='$FONT':text='$(dt "$sub")':fontcolor=0xF2F6F5@0.72:fontsize=$ss:x=w*0.08:y=h*0.86-$ss"
  fi
  "${FF[@]}" -f lavfi -i "gradients=s=${w}x${h}:c0=${c0}:c1=${c1}:x0=0:y0=0:x1=0:y1=${h}:nb_colors=2" \
    -frames:v 1 -vf "$vf" -q:v 3 "$out" \
  || "${FF[@]}" -f lavfi -i "gradients=s=${w}x${h}:c0=${c0}:c1=${c1}:x0=0:y0=0:x1=0:y1=${h}:nb_colors=2" -frames:v 1 -q:v 3 "$out"
}

# clip <out.mkv> — 2 s test pattern, 320x180 H.264 + AAC (French audio tag).
clip() {
  "${FF[@]}" -f lavfi -i "testsrc=size=320x180:rate=24:duration=2" -f lavfi -i "sine=frequency=440:duration=2" \
    -c:v libx264 -preset ultrafast -pix_fmt yuv420p -c:a aac -b:a 64k -shortest \
    -metadata:s:a:0 language=fre "$1"
}

# ——— Films ———————————————————————————————————————————————————————————————
# title|year|tagline|genres(,)|director|actors(,)|runtime|mpaa|c0|c1|plot
MOVIES=(
"Les Heures claires|2019|Certains étés ne finissent jamais.|Drame,Mystère|Hélène Varga|Léa Morel,Julien Ferrant,Inès Barre|132|Tous publics|0x3B2A4A|0x0C1220|Au cœur d'un été immobile, une photographe revient dans la maison de son enfance et découvre le portrait d'une femme que personne ne veut nommer."
"Marée basse|2021|La mer se retire. Les secrets aussi.|Drame,Aventure|Jonas Lefèvre|Marc Aubert,Clara Nguyen|108|Tous publics|0x2C4A5A|0x0A1A24|Sur une île de la Manche, un gardien de phare à la retraite et une biologiste venue du continent attendent la plus grande marée du siècle."
"The Quiet Orbit|2023|Six months. One signal.|Science-fiction,Thriller|Maya Holloway|Ethan Cole,Priya Raman,Tom Ashby|121|PG-13|0x1D3557|0x060B18|Alone aboard a decommissioned relay station, an engineer starts receiving messages timestamped from next week."
"Archipel|2018|Chaque île garde sa mémoire.|Science-fiction,Aventure|Sofia Brandt|Nora Lind,Paul Mercier|115|Tous publics|0x24476B|0x081424|Une cartographe part relever les côtes d'un archipel qui n'apparaît sur aucune carte ancienne."
"Hautes Terres|2020|Là-haut, personne ne vous entend partir.|Aventure,Drame|Antoine Roux|Camille Duval,Yann Kervella|126|Tous publics|0x2F4A3A|0x0A160F|Deux frères que tout oppose traversent les Highlands pour disperser les cendres de leur père."
"Le Dernier Été|2016|On ne revient jamais vraiment.|Romance,Drame|Claire Delmas|Louise Bernard,Hugo Lambert|97|Tous publics|0x5A3E2B|0x1A0F0A|Dans une station balnéaire hors saison, deux anciens amants se retrouvent le temps d'un dernier week-end."
"Night Shift at Kessler's|2022|Open all night. Leave before dawn.|Horreur,Thriller|Dean Marlow|Rosa Diaz,Kevin Park|94|R|0x3A2030|0x0D070C|A diner on Route 9 hires a new waitress for the graveyard shift; the regulars have not aged in forty years."
"La Brigade du fleuve|2017|Le courant ne pardonne rien.|Policier,Thriller|Romain Castel|Élodie Marchand,Samir Haddad|110|Déconseillé aux moins de 12 ans|0x2B3A4A|0x0A0F16|Une unité fluviale de la police de Lyon enquête sur une série de disparitions le long du Rhône."
"Dune de sel|2015|Le désert efface tout, sauf la vérité.|Aventure,Drame|Nadia Benali|Karim Sabri,Lucie Fontaine|124|Tous publics|0x6A4A30|0x1E120A|Une géologue et son guide touareg cherchent une caravane disparue au milieu des salines."
"Paper Lanterns|2024|Some lights are meant to be followed.|Animation,Famille|Kenji Arata|Mei Tanaka,Oliver Grant|89|G|0x4A3A5A|0x120E1C|A shy lantern-maker's apprentice follows a runaway paper lantern through a city that only exists at night."
"Les Veilleurs du port|2014|Ils ne dorment jamais.|Policier,Drame|Marc Olivier|François Leclerc,Anne Vidal|103|Tous publics|0x2A3F55|0x080F18|À Marseille, deux dockers de nuit découvrent un conteneur qui ne figure sur aucun manifeste."
"Echoes of Ember Hill|2012|The fire never really went out.|Mystère,Fantastique|Grace Whitfield|Henry Wells,Ada Brooks|117|PG-13|0x5A3328|0x170B08|Years after a wildfire, a journalist returns to her hometown where the burned forest has begun to whisper."
)

echo "→ Films"
SEED=0
for m in "${MOVIES[@]}"; do
  SEED=$((SEED + 1))
  IFS='|' read -r title year tagline genres director actors runtime mpaa c0 c1 plot <<<"$m"
  dir="$FILMS/$title ($year)"; mkdir -p "$dir"
  [[ -f "$dir/$title ($year).mkv" ]] || clip "$dir/$title ($year).mkv"
  {
    echo '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
    echo '<movie>'
    echo "  <title>$(xml "$title")</title>"
    echo "  <year>$year</year>"
    echo "  <tagline>$(xml "$tagline")</tagline>"
    echo "  <plot>$(xml "$plot")</plot>"
    echo "  <runtime>$runtime</runtime>"
    echo "  <mpaa>$(xml "$mpaa")</mpaa>"
    echo "  <premiered>$year-0$(( (year % 9) + 1 ))-1$(( year % 9 ))</premiered>"
    echo "  <rating>$(( 60 + year % 30 ))</rating>"
    IFS=',' read -ra gs <<<"$genres"; for g in "${gs[@]}"; do echo "  <genre>$(xml "$g")</genre>"; done
    echo "  <director>$(xml "$director")</director>"
    echo "  <credits>$(xml "$director")</credits>"
    IFS=',' read -ra as <<<"$actors"; i=0
    for a in "${as[@]}"; do echo "  <actor><name>$(xml "$a")</name><role>Rôle $((++i))</role><order>$i</order></actor>"; done
    echo '</movie>'
  } >"$dir/movie.nfo"
  [[ -f "$dir/poster.jpg" ]] || art "$dir/poster.jpg" 600 900 "$c0" "$c1" "$title" "$director · $year" $SEED
  [[ -f "$dir/fanart.jpg" ]] || art "$dir/fanart.jpg" 1920 1080 "$c0" "$c1" "" "" $SEED
done

# ——— Séries ——————————————————————————————————————————————————————————————
echo "→ Séries"
series() {
  local title=$1 year=$2 status=$3 genres=$4 c0=$5 c1=$6 plot=$7; shift 7
  local dir="$SERIES/$title ($year)"; mkdir -p "$dir"
  {
    echo '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
    echo '<tvshow>'
    echo "  <title>$(xml "$title")</title><year>$year</year><status>$status</status>"
    echo "  <plot>$(xml "$plot")</plot><premiered>$year-09-14</premiered><mpaa>Tous publics</mpaa>"
    IFS=',' read -ra gs <<<"$genres"; for g in "${gs[@]}"; do echo "  <genre>$(xml "$g")</genre>"; done
    echo '</tvshow>'
  } >"$dir/tvshow.nfo"
  [[ -f "$dir/poster.jpg" ]] || art "$dir/poster.jpg" 600 900 "$c0" "$c1" "$title" "Série · $year"
  [[ -f "$dir/fanart.jpg" ]] || art "$dir/fanart.jpg" 1920 1080 "$c0" "$c1" "" ""
  local s e name
  for s in 1 2; do
    local sd="$dir/Season 0$s"; mkdir -p "$sd"
    [[ -f "$sd/../season0$s-poster.jpg" ]] || art "$dir/season0$s-poster.jpg" 600 900 "$c0" "$c1" "Saison $s" "$title"
    for e in 1 2 3; do
      name=$1; shift
      local base="$sd/$title - S0${s}E0${e}"
      [[ -f "$base.mkv" ]] || clip "$base.mkv"
      cat >"$base.nfo" <<NFO
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<episodedetails>
  <title>$(xml "$name")</title><season>$s</season><episode>$e</episode>
  <aired>$((year + s - 1))-10-0$e</aired><runtime>$(( 42 + e * 3 ))</runtime>
  <plot>$(xml "$name") — épisode $e de la saison $s de « $title ».</plot>
</episodedetails>
NFO
      [[ -f "$base-thumb.jpg" ]] || art "$base-thumb.jpg" 960 540 "$c1" "$c0" "" ""
    done
  done
}
series "Les Veilleurs" 2021 "Continuing" "Drame,Mystère" 0x2A2448 0x0A0A18 \
  "Sur la côte bretonne, les gardiens d'un phare automatisé reçoivent des signaux d'un navire disparu en 1952." \
  "Le Phare de Kéréon" "Brume" "Le Registre" "Signaux" "La Relève" "Grande Marée"
series "Hautes Fréquences" 2019 "Ended" "Comédie,Drame" 0x3A4A3A 0x0E140E \
  "Une petite radio associative du Jura tente de survivre à l'arrivée d'un géant du streaming." \
  "Antenne" "Direct" "Rediffusion" "Silence radio" "Ondes courtes" "Dernière émission"

# ——— Musique —————————————————————————————————————————————————————————————
echo "→ Musique"
album() {
  local artist=$1 album=$2 year=$3 c0=$4 c1=$5; shift 5
  local dir="$MUSIC/$artist/$album ($year)"; mkdir -p "$dir"
  [[ -f "$dir/cover.jpg" ]] || art "$dir/cover.jpg" 800 800 "$c0" "$c1" "$album" "$artist"
  [[ -f "$MUSIC/$artist/folder.jpg" ]] || art "$MUSIC/$artist/folder.jpg" 800 800 "$c1" "$c0" "$artist" ""
  local n=0 t
  for t in "$@"; do
    n=$((n + 1))
    local f; f=$(printf '%s/%02d - %s.flac' "$dir" "$n" "$t")
    [[ -f "$f" ]] && continue
    "${FF[@]}" -f lavfi -i "sine=frequency=$((220 + n * 55)):duration=$((3 + n % 3))" -c:a flac \
      -metadata title="$t" -metadata artist="$artist" -metadata album_artist="$artist" \
      -metadata album="$album" -metadata track="$n" -metadata date="$year" -metadata genre="Électro" "$f"
  done
}
album "Lune Pâle" "Côte sauvage" 2020 0x2E4A5A 0x0A141C "Ouverture" "Vents d'ouest" "Les Falaises" "Retour au port"
album "Lune Pâle" "Nocturnes" 2023 0x3A2E5A 0x0C0A1C "Minuit" "Lanternes" "Dernier métro"
album "Harbor Lights" "Slow Tide" 2021 0x5A4A3A 0x1A140E "Low Water" "Driftwood" "Harbor Lights" "Undertow" "Morning Fog"

cat >"$DEST/aether.toml" <<TOML
# Configuration de démonstration (aucun appel réseau).
[metadata]
language = "fr-FR"
disable_musicbrainz = true
TOML

echo "Arborescence prête dans $DEST"
echo "  Films   : $FILMS"
echo "  Séries  : $SERIES"
echo "  Musique : $MUSIC"
echo "  Config  : $DEST/aether.toml"
