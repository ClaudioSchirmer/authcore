// Tests for the token DTOs.
//
// The mapping itself is dull; two properties are not.
//
// EMPTY LISTS MUST RENDER AS `[]`, NEVER `null`. A client iterating groups or
// permissions should not have to guard, and "this user is in no groups" is an
// empty list rather than a missing field. Go's nil slice marshals to null, so
// this only holds because the projection forces it — and only stays holding
// because these tests check the marshalled bytes rather than the Go value.
//
// THE BODY MIRRORS THE TOKEN. The response's permission list is the same
// restricted set the access token carries, so a client never has to decode a JWT
// to know what it may attempt.

package requests

import (
	"encoding/json"
	cmddtos "github.com/ClaudioSchirmer/authcore/internal/application/commands/dtos"
	"github.com/ClaudioSchirmer/authcore/internal/web/requests/dtos"
	"strings"
	"testing"
)

func TestIssueTokenRequest_ToCommand(t *testing.T) {
	cmd := IssueTokenRequest{Email: "ada@acme.test", Password: "secret"}.ToCommand()
	if cmd.Email != "ada@acme.test" || cmd.Password != "secret" {
		t.Errorf("mapped to %+v", cmd)
	}
}

func TestRefreshTokenRequest_ToCommand(t *testing.T) {
	cmd := RefreshTokenRequest{RefreshToken: "opaque"}.ToCommand()
	if cmd.RefreshToken != "opaque" {
		t.Errorf("mapped to %+v", cmd)
	}
}

func TestTokenResponse_FromResult(t *testing.T) {
	result := cmddtos.TokenResult{
		AccessToken:      "access",
		TokenType:        "Bearer",
		ExpiresAt:        1700000000,
		RefreshToken:     "refresh",
		RefreshExpiresAt: 1700003600,
		User: cmddtos.AuthenticatedUserResult{
			ID:                 "user-1",
			Name:               "Ada Lovelace",
			Email:              "ada@acme.test",
			Status:             "active",
			MustChangePassword: true,
			TenantID:           "tenant-1",
			TenantWorkspace:    "acme",
			Groups:             []cmddtos.NamedGrantResult{{Key: "eng", Name: "Engineering"}},
			Roles:              []cmddtos.NamedGrantResult{{Key: "viewer"}},
			Permissions:        []string{"user:change-password"},
		},
	}

	resp := TokenResponse{}.FromResult(result)

	if resp.AccessToken != "access" || resp.RefreshToken != "refresh" {
		t.Errorf("tokens not carried through: %+v", resp)
	}
	if resp.ExpiresAt != 1700000000 || resp.RefreshExpiresAt != 1700003600 {
		t.Errorf("expiries not carried through: %+v", resp)
	}
	if !resp.User.MustChangePassword {
		t.Error("the client is not told to route to the change screen")
	}
	if resp.User.TenantWorkspace != "acme" {
		t.Errorf("workspace lost: %+v", resp.User)
	}
	if len(resp.User.Groups) != 1 || resp.User.Groups[0].Name != "Engineering" {
		t.Errorf("groups = %+v — the display names are what the body exists to carry", resp.User.Groups)
	}
	// An inherited role has no display name, and the DTO must not invent one.
	if len(resp.User.Roles) != 1 || resp.User.Roles[0].Name != "" {
		t.Errorf("roles = %+v", resp.User.Roles)
	}
}

// THE ONE THAT ACTUALLY BITES. A nil slice marshals to null; a client doing
// `for (const g of user.groups)` then throws instead of iterating nothing.
func TestTokenResponse_EmptyCollectionsMarshalAsArrays(t *testing.T) {
	resp := TokenResponse{}.FromResult(cmddtos.TokenResult{})

	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	body := string(encoded)

	for _, field := range []string{`"groups":[]`, `"roles":[]`, `"permissions":[]`, `"claims":{}`} {
		if !strings.Contains(body, field) {
			t.Errorf("expected %s in the body, got:\n%s\n\nnull here makes a client guard for it", field, body)
		}
	}
	if strings.Contains(body, "null") {
		t.Errorf("no field of this response should ever be null:\n%s", body)
	}
}

// The tenant claims reach the wire under their own names and IN THEIR OWN JSON
// TYPES — a number as a number, a bool as a bool. Rendering them as strings
// would make every consumer parse what the definition already declared.
func TestTokenResponse_CarriesTenantClaimsInTheirDeclaredTypes(t *testing.T) {
	resp := TokenResponse{}.FromResult(cmddtos.TokenResult{
		User: cmddtos.AuthenticatedUserResult{
			Claims: map[string]any{
				"x_cost_center":  float64(1000),
				"x_beta_enabled": true,
				"x_region":       "emea",
			},
		},
	})

	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	body := string(encoded)
	for _, fragment := range []string{`"x_cost_center":1000`, `"x_beta_enabled":true`, `"x_region":"emea"`} {
		if !strings.Contains(body, fragment) {
			t.Errorf("expected %s in the body, got:\n%s", fragment, body)
		}
	}
}

// A role's display name is omitted rather than sent empty — an inherited role
// genuinely has none, and `"name":""` would read as a role literally called "".
func TestNamedGrantResponse_OmitsAnAbsentName(t *testing.T) {
	encoded, err := json.Marshal(dtos.NamedGrantResponse{Key: "viewer"})
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if got := string(encoded); got != `{"key":"viewer"}` {
		t.Errorf("got %s, want the name omitted entirely", got)
	}
}

// Each credential appears in exactly ONE field. A response that echoed a token
// into a second place would double the number of logs, caches and error reports
// that end up holding it.
//
// The sentinels deliberately do not overlap the field NAMES — an earlier version
// of this test used "access" and "refresh" and failed against a perfectly correct
// response, because those strings also occur inside "accessToken".
func TestTokenResponse_CarriesEachSecretExactlyOnce(t *testing.T) {
	resp := TokenResponse{}.FromResult(cmddtos.TokenResult{
		AccessToken:  "SENTINEL-JWT-VALUE",
		RefreshToken: "SENTINEL-OPAQUE-VALUE",
	})
	encoded, _ := json.Marshal(resp)
	body := string(encoded)

	if strings.Count(body, "SENTINEL-JWT-VALUE") != 1 {
		t.Errorf("the access token appears more than once:\n%s", body)
	}
	if strings.Count(body, "SENTINEL-OPAQUE-VALUE") != 1 {
		t.Errorf("the refresh token appears more than once:\n%s", body)
	}
}
