// Tests for the machine sign-in's wire shapes.
//
// WHAT THIS FILE PROTECTS is the CONTRACT, which is the thing a caller writes code
// against and cannot see change: the JSON names, the fact that no refresh
// credential appears anywhere in the response, and that a collection renders as `[]`
// or `{}` rather than `null` so a client iterating it never has to guard first.

package requests

import (
	"encoding/json"
	cmdutils "github.com/ClaudioSchirmer/authcore/internal/application/commands/utils"
	"strings"
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
)

func TestIssueClientTokenRequestMapsToTheCommand(t *testing.T) {
	cmd := IssueClientTokenRequest{
		ClientID:     "018f2c7e-9a41-7b3d-8c52-2f7a1d9e4b60",
		ClientSecret: "acs_secret",
	}.ToCommand()

	if cmd.ClientID != "018f2c7e-9a41-7b3d-8c52-2f7a1d9e4b60" || cmd.ClientSecret != "acs_secret" {
		t.Errorf("the body did not reach the command intact: %+v", cmd)
	}
}

// The request's wire names are the contract, and they are asserted against the
// rendered JSON rather than against the struct tags — a tag typo compiles.
func TestIssueClientTokenRequestWireNames(t *testing.T) {
	raw, err := json.Marshal(IssueClientTokenRequest{ClientID: "id", ClientSecret: "secret"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("the body carries %d fields (%v); two, and no third — no grantType, no tenant", len(got), got)
	}
	for _, key := range []string{"clientId", "clientSecret"} {
		if _, ok := got[key]; !ok {
			t.Errorf("the body does not carry %q", key)
		}
	}
}

// THE RESPONSE CARRIES NO REFRESH CREDENTIAL, anywhere, under any spelling.
//
// This is the assertion that makes RFC 6749 §4.4.3 a property of the code rather
// than a paragraph in a comment: a client-credentials grant issues no refresh token,
// because the client's secret already IS its long-lived credential. Somebody
// "unifying" this shape with the user one would have to break this test.
func TestClientTokenResponseCarriesNoRefreshCredential(t *testing.T) {
	raw, err := json.Marshal(ClientTokenResponse{}.FromResult(commands.ClientTokenResult{}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, forbidden := range []string{"refreshToken", "refreshExpiresAt"} {
		if _, present := got[forbidden]; present {
			t.Errorf("the response carries %q; this grant issues no refresh credential", forbidden)
		}
	}
	for _, required := range []string{"accessToken", "tokenType", "expiresAt", "client"} {
		if _, present := got[required]; !present {
			t.Errorf("the response does not carry %q", required)
		}
	}
}

// The profile's wire names, and the fact that the subject is `client` rather than
// `user`.
func TestClientTokenResponseProfileWireNames(t *testing.T) {
	raw, err := json.Marshal(ClientTokenResponse{}.FromResult(commands.ClientTokenResult{}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var envelope struct {
		Client map[string]any `json:"client"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"id", "name", "status", "tenantId", "tenantWorkspace", "roles", "permissions", "claims"} {
		if _, ok := envelope.Client[key]; !ok {
			t.Errorf("the profile does not carry %q", key)
		}
	}
	// The three a machine has no business carrying — the same absences the token
	// asserts, held at the wire too so the two cannot drift apart.
	for _, forbidden := range []string{"email", "groups", "mustChangePassword"} {
		if _, present := envelope.Client[forbidden]; present {
			t.Errorf("the profile carries %q; a machine has no such thing", forbidden)
		}
	}
}

// Empty collections render as `[]` and `{}`, never `null`.
//
// A caller ranging over roles or reading a key out of claims should not have to
// guard first: "this integration holds no roles" is an empty list, not a missing
// one.
func TestClientTokenResponseRendersEmptyCollectionsNotNull(t *testing.T) {
	raw, err := json.Marshal(ClientTokenResponse{}.FromResult(commands.ClientTokenResult{}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"roles":[]`, `"permissions":[]`, `"claims":{}`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the response does not render %s; got %s", want, raw)
		}
	}
}

// Everything the result carries reaches the wire.
func TestClientTokenResponseCarriesTheResultThrough(t *testing.T) {
	result := commands.ClientTokenResult{
		AccessToken: "token",
		TokenType:   "Bearer",
		ExpiresAt:   5000,
		Client: commands.AuthenticatedClientResult{
			ID:              "client-id",
			Name:            "Billing integration",
			Status:          "active",
			TenantID:        "tenant-id",
			TenantWorkspace: "acme",
			Roles:           []cmdutils.NamedGrantResult{{Key: "billing-admin", Name: "Billing admin"}},
			Permissions:     []string{"user:read"},
			Claims:          map[string]any{"x_region": "sa-east-1"},
		},
	}
	got := ClientTokenResponse{}.FromResult(result)

	if got.AccessToken != "token" || got.TokenType != "Bearer" || got.ExpiresAt != 5000 {
		t.Errorf("the token half did not survive: %+v", got)
	}
	if got.Client.ID != "client-id" || got.Client.Name != "Billing integration" ||
		got.Client.Status != "active" || got.Client.TenantWorkspace != "acme" {
		t.Errorf("the profile did not survive: %+v", got.Client)
	}
	if len(got.Client.Roles) != 1 || got.Client.Roles[0].Key != "billing-admin" ||
		got.Client.Roles[0].Name != "Billing admin" {
		t.Errorf("the roles did not survive: %+v", got.Client.Roles)
	}
	if len(got.Client.Permissions) != 1 || got.Client.Permissions[0] != "user:read" {
		t.Errorf("the permissions did not survive: %+v", got.Client.Permissions)
	}
	if got.Client.Claims["x_region"] != "sa-east-1" {
		t.Errorf("the tenant claims did not survive: %+v", got.Client.Claims)
	}
}
