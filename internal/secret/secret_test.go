package secret

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripAndPersistence(t *testing.T) {
	dir := t.TempDir()
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	type creds struct{ Password string }
	sealed, err := b.SealJSON(creds{"hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(filepath.Join(dir, "secret.key")); st.Mode().Perm() != 0o600 {
		t.Errorf("key perms = %v", st.Mode().Perm())
	}
	b2, _ := Open(dir) // same key reloaded
	var got creds
	if err := b2.OpenJSON(sealed, &got); err != nil || got.Password != "hunter2" {
		t.Fatalf("got %+v, %v", got, err)
	}
	sealed[len(sealed)-1] ^= 1
	if err := b2.OpenJSON(sealed, &got); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
	other, _ := Open(t.TempDir())
	if err := other.OpenJSON(sealed, &got); err == nil {
		t.Fatal("other key must fail")
	}
}
