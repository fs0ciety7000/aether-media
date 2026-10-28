package agentproto

import (
	"bytes"
	"strings"
	"testing"
)

func TestFingerprintForms(t *testing.T) {
	fp := Fingerprint([]byte("cert"))
	if len(fp) != 64 || strings.ToLower(fp) != fp {
		t.Fatal(fp)
	}
	disp := FormatFingerprint(fp)
	if len(disp) != 64+15 || strings.Count(disp, " ") != 15 || strings.ToUpper(disp) != disp {
		t.Fatalf("display %q", disp)
	}
	colons := ""
	for i := 0; i < len(fp); i += 2 {
		if i > 0 {
			colons += ":"
		}
		colons += strings.ToUpper(fp[i : i+2])
	}
	for _, in := range []string{fp, disp, colons, "sha256:" + fp, " " + strings.ToUpper(fp) + "\t"} {
		if n, err := NormalizeFingerprint(in); err != nil || n != fp {
			t.Errorf("%q → %q %v", in, n, err)
		}
	}
	for _, bad := range []string{"abc", fp + "00", strings.Replace(fp, "a", "g", 1), "sha1:" + fp} {
		if _, err := NormalizeFingerprint(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if n, err := NormalizeFingerprint(""); n != "" || err != nil {
		t.Fatal("empty")
	}
}

func TestProofsBindEverything(t *testing.T) {
	nonce := bytes.Repeat([]byte{1}, NonceLen)
	k := PairKey("K7PQ4XRM", nonce)
	if bytes.Equal(k, PairKey("K7PQ4XRN", nonce)) || bytes.Equal(k, PairKey("K7PQ4XRM", bytes.Repeat([]byte{2}, NonceLen))) {
		t.Fatal("key ignores code or nonce")
	}
	p := ServerProof(k, "fp", nonce, "srv")
	for _, o := range [][]byte{
		ServerProof(k, "fq", nonce, "srv"),
		ServerProof(k, "fp", nonce, "srw"),
		ServerProof(k, "fp", bytes.Repeat([]byte{3}, NonceLen), "srv"),
		AgentProof(k, "fp", nonce, "srv"), // roles are separated
	} {
		if Equal(p, o) {
			t.Fatal("proof does not bind its inputs")
		}
	}
	// Field separation: ("a","bc") differs from ("ab","c").
	if Equal(ServerProof(k, "a", nonce, "bc"), ServerProof(k, "ab", nonce, "c")) {
		t.Fatal("ambiguous encoding")
	}
}
