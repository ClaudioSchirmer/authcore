// Tests for the refresh-token store.
//
// ONE THING IS TESTED HERE, and it is the only one that can be: the hash form.
// This store mirrors authcore's hashing because the framework's hashRefreshValue
// is unexported, so if a release ever changed it every rotation in production
// would answer 401 and nothing would say why. The mirror is pinned against a REAL
// round trip through the framework's own Issuer rather than against a constant
// somebody copied over.
//
// EVERYTHING ELSE MOVED TO refresh_token_store_live_test.go, and the reason is
// worth stating so nobody brings it back. While the store wrote its own SQL there
// was something here to assert: the text of a statement was a decision this file
// made. The statements are the framework's now, so a unit test could only fake the
// repository and check that Save calls Insert and MarkUsed calls Update — which is
// the implementation copied into the assertion. It would break when the code was
// changed correctly and pass when the database disagreed with the schema, which is
// exactly backwards. What is worth proving is that a saved token comes back, that
// a burn hits one row, that a revoke hits the family, and that the sweep spares
// the grace margin — and every one of those is a statement against a real
// database.

package infra

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"testing"
	"time"

	"github.com/ClaudioSchirmer/omnicore/web/authcore"
)

func TestSubjectForRefreshToken_HashMirrorsTheFramework(t *testing.T) {
	captured := &capturingStore{}
	issuer, err := authcore.NewIssuer(authcore.IssuerOptions{
		SelfURL:         "http://localhost",
		Audience:        []string{"authcore"},
		TokenTTL:        time.Minute,
		MaxTokenTTL:     time.Hour,
		RefreshTokenTTL: time.Hour,
		Keys: []authcore.SigningKey{{
			KID:        "test",
			Algorithm:  "RS256",
			PrivatePEM: testSigningKeyPEM(t),
			State:      authcore.KeyCurrent,
		}},
		RefreshStore: captured,
	})
	if err != nil {
		t.Fatalf("building the issuer: %v", err)
	}

	_, refresh, err := issuer.IssueWithRefresh(context.Background(), authcore.TokenRequest{Subject: "user-1"})
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	sum := sha256.Sum256([]byte(refresh.Value))
	mirrored := hex.EncodeToString(sum[:])
	if mirrored != captured.saved.Hash {
		t.Fatalf("this store hashes a refresh value to %q, the framework stored %q — "+
			"SubjectForRefreshToken would find nothing and every rotation would answer 401",
			mirrored, captured.saved.Hash)
	}
	if captured.saved.Subject != "user-1" {
		t.Errorf("subject = %q, want the one the token was minted for", captured.saved.Subject)
	}
}

// capturingStore is the minimal port implementation the pin above needs.
type capturingStore struct{ saved authcore.RefreshTokenRecord }

func (s *capturingStore) Save(_ context.Context, rec authcore.RefreshTokenRecord) error {
	s.saved = rec
	return nil
}
func (s *capturingStore) Lookup(context.Context, string) (authcore.RefreshTokenRecord, error) {
	return authcore.RefreshTokenRecord{}, authcore.ErrRefreshTokenNotFound
}
func (s *capturingStore) MarkUsed(context.Context, string) error     { return nil }
func (s *capturingStore) RevokeFamily(context.Context, string) error { return nil }

// testSigningKeyPEM generates a throwaway PKCS#8 RSA key.
//
// Generated rather than embedded: a committed private key in a test fixture is a
// committed private key, and the ones that leak are always the ones somebody was
// sure did not matter.
func testSigningKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating a test key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("encoding the test key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}
