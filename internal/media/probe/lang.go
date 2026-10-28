package probe

import "strings"

// iso6391 maps ISO 639-1 two-letter codes to ISO 639-2/B.
var iso6391 = map[string]string{
	"aa": "aar", "ab": "abk", "ae": "ave", "af": "afr", "ak": "aka", "am": "amh", "an": "arg",
	"ar": "ara", "as": "asm", "av": "ava", "ay": "aym", "az": "aze", "ba": "bak", "be": "bel",
	"bg": "bul", "bh": "bih", "bi": "bis", "bm": "bam", "bn": "ben", "bo": "tib", "br": "bre",
	"bs": "bos", "ca": "cat", "ce": "che", "ch": "cha", "co": "cos", "cr": "cre", "cs": "cze",
	"cu": "chu", "cv": "chv", "cy": "wel", "da": "dan", "de": "ger", "dv": "div", "dz": "dzo",
	"ee": "ewe", "el": "gre", "en": "eng", "eo": "epo", "es": "spa", "et": "est", "eu": "baq",
	"fa": "per", "ff": "ful", "fi": "fin", "fj": "fij", "fo": "fao", "fr": "fre", "fy": "fry",
	"ga": "gle", "gd": "gla", "gl": "glg", "gn": "grn", "gu": "guj", "gv": "glv", "ha": "hau",
	"he": "heb", "hi": "hin", "ho": "hmo", "hr": "hrv", "ht": "hat", "hu": "hun", "hy": "arm",
	"hz": "her", "ia": "ina", "id": "ind", "ie": "ile", "ig": "ibo", "ii": "iii", "ik": "ipk",
	"io": "ido", "is": "ice", "it": "ita", "iu": "iku", "ja": "jpn", "jv": "jav", "ka": "geo",
	"kg": "kon", "ki": "kik", "kj": "kua", "kk": "kaz", "kl": "kal", "km": "khm", "kn": "kan",
	"ko": "kor", "kr": "kau", "ks": "kas", "ku": "kur", "kv": "kom", "kw": "cor", "ky": "kir",
	"la": "lat", "lb": "ltz", "lg": "lug", "li": "lim", "ln": "lin", "lo": "lao", "lt": "lit",
	"lu": "lub", "lv": "lav", "mg": "mlg", "mh": "mah", "mi": "mao", "mk": "mac", "ml": "mal",
	"mn": "mon", "mr": "mar", "ms": "may", "mt": "mlt", "my": "bur", "na": "nau", "nb": "nob",
	"nd": "nde", "ne": "nep", "ng": "ndo", "nl": "dut", "nn": "nno", "no": "nor", "nr": "nbl",
	"nv": "nav", "ny": "nya", "oc": "oci", "oj": "oji", "om": "orm", "or": "ori", "os": "oss",
	"pa": "pan", "pi": "pli", "pl": "pol", "ps": "pus", "pt": "por", "qu": "que", "rm": "roh",
	"rn": "run", "ro": "rum", "ru": "rus", "rw": "kin", "sa": "san", "sc": "srd", "sd": "snd",
	"se": "sme", "sg": "sag", "si": "sin", "sk": "slo", "sl": "slv", "sm": "smo", "sn": "sna",
	"so": "som", "sq": "alb", "sr": "srp", "ss": "ssw", "st": "sot", "su": "sun", "sv": "swe",
	"sw": "swa", "ta": "tam", "te": "tel", "tg": "tgk", "th": "tha", "ti": "tir", "tk": "tuk",
	"tl": "tgl", "tn": "tsn", "to": "ton", "tr": "tur", "ts": "tso", "tt": "tat", "tw": "twi",
	"ty": "tah", "ug": "uig", "uk": "ukr", "ur": "urd", "uz": "uzb", "ve": "ven", "vi": "vie",
	"vo": "vol", "wa": "wln", "wo": "wol", "xh": "xho", "yi": "yid", "yo": "yor", "za": "zha",
	"zh": "chi", "zu": "zul",
	// Deprecated two-letter codes still found in the wild.
	"iw": "heb", "in": "ind", "ji": "yid", "mo": "rum",
}

// terminologic maps ISO 639-2/T (and a few deprecated or non-standard)
// three-letter codes to their ISO 639-2/B equivalent.
var terminologic = map[string]string{
	"sqi": "alb", "hye": "arm", "eus": "baq", "mya": "bur", "zho": "chi", "ces": "cze",
	"nld": "dut", "fra": "fre", "kat": "geo", "deu": "ger", "ell": "gre", "isl": "ice",
	"mkd": "mac", "mri": "mao", "msa": "may", "fas": "per", "ron": "rum", "slk": "slo",
	"bod": "tib", "cym": "wel",
	"scc": "srp", "scr": "hrv", "jap": "jpn", "cmn": "chi", "yue": "chi",
}

// languageNames maps common language names (English and native) that some
// muxers write instead of codes.
var languageNames = map[string]string{
	"english": "eng", "french": "fre", "francais": "fre", "français": "fre", "german": "ger",
	"deutsch": "ger", "spanish": "spa", "espanol": "spa", "español": "spa", "castellano": "spa",
	"italian": "ita", "italiano": "ita", "japanese": "jpn", "chinese": "chi", "korean": "kor",
	"russian": "rus", "portuguese": "por", "português": "por", "dutch": "dut",
	"nederlands": "dut", "swedish": "swe", "norwegian": "nor", "danish": "dan", "finnish": "fin",
	"polish": "pol", "czech": "cze", "hungarian": "hun", "greek": "gre", "turkish": "tur",
	"arabic": "ara", "hebrew": "heb", "hindi": "hin", "thai": "tha", "vietnamese": "vie",
	"indonesian": "ind", "ukrainian": "ukr", "romanian": "rum", "bulgarian": "bul",
	"croatian": "hrv", "serbian": "srp", "slovak": "slo", "slovenian": "slv", "catalan": "cat",
}

// NormaliseLanguage converts a language tag to a lowercase ISO 639-2/B code.
// It accepts ISO 639-1 ("fr"), ISO 639-2/T ("fra"), BCP 47-ish tags
// ("fr-CA", "pt_BR") and common names ("French"). Undetermined or
// unrecognised values ("und", "unknown", "") yield "".
func NormaliseLanguage(tag string) string {
	t := strings.ToLower(strings.TrimSpace(tag))
	if t == "" {
		return ""
	}
	if v, ok := languageNames[t]; ok {
		return v
	}
	if i := strings.IndexAny(t, "-_"); i > 0 {
		t = t[:i]
	}
	for _, r := range t {
		if r < 'a' || r > 'z' {
			return ""
		}
	}
	switch len(t) {
	case 2:
		return iso6391[t]
	case 3:
		switch t {
		case "und", "unk", "zxx", "mis", "xxx":
			return ""
		}
		if b, ok := terminologic[t]; ok {
			return b
		}
		return t
	}
	return ""
}
