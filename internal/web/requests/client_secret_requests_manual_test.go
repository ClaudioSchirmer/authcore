// Tests for the rotation's wire shapes.
//
// Two mappers and four assertions, and the one that earns its place is the
// pointer: absent and zero are different requests, and a mapper that collapsed
// them would turn every unstated rotation into an immediate cutover with nothing
// reporting it.

package requests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

func TestRotateRequestCarriesAnOmittedWindowAsAbsent(t *testing.T) {
	var r RotateClientSecretRequest
	if err := json.Unmarshal([]byte(`{}`), &r); err != nil {
		t.Fatalf("an empty body did not decode: %v", err)
	}
	cmd := r.ToCommand()
	if cmd.GracePeriodSeconds != nil {
		t.Fatalf("an omitted window decoded as %d; absent must stay absent so the handler can apply the default", *cmd.GracePeriodSeconds)
	}
}

// TestRotateRequestKeepsAnExplicitZero is the leaked-credential path, and it is
// the whole reason the field is a pointer.
func TestRotateRequestKeepsAnExplicitZero(t *testing.T) {
	var r RotateClientSecretRequest
	if err := json.Unmarshal([]byte(`{"gracePeriodSeconds":0}`), &r); err != nil {
		t.Fatalf("body did not decode: %v", err)
	}
	cmd := r.ToCommand()
	if cmd.GracePeriodSeconds == nil {
		t.Fatal("an explicit zero decoded as absent; a leaked secret would keep working for a day")
	}
	if *cmd.GracePeriodSeconds != 0 {
		t.Fatalf("an explicit zero decoded as %d", *cmd.GracePeriodSeconds)
	}
}

func TestRotateRequestPassesAWindowThrough(t *testing.T) {
	var r RotateClientSecretRequest
	if err := json.Unmarshal([]byte(`{"gracePeriodSeconds":3600}`), &r); err != nil {
		t.Fatalf("body did not decode: %v", err)
	}
	cmd := r.ToCommand()
	if cmd.GracePeriodSeconds == nil || *cmd.GracePeriodSeconds != 3600 {
		t.Fatalf("the window did not survive the mapper: %v", cmd.GracePeriodSeconds)
	}
}

func TestRotateResponseRendersTheResult(t *testing.T) {
	changed := time.Date(2026, 8, 26, 14, 3, 11, 0, time.UTC)
	expires := changed.Add(24 * time.Hour)

	got := RotateClientSecretResponse{}.FromResult(commands.RotateClientSecretResult{
		ID:                      domain.NewID("7b3c1f10-3c7e-4a8d-9f0e-9d2a8e6d4b51"),
		Secret:                  "acs_8xQvR2mK9dLpN4wZ7tYcB1hJ6sF3gA5eU0iO8rTvXyM",
		SecretChangedAt:         changed,
		PreviousSecretExpiresAt: &expires,
	})

	if got.ID != "7b3c1f10-3c7e-4a8d-9f0e-9d2a8e6d4b51" {
		t.Fatalf("the id crossed as %q", got.ID)
	}
	if got.Secret != "acs_8xQvR2mK9dLpN4wZ7tYcB1hJ6sF3gA5eU0iO8rTvXyM" {
		t.Fatalf("the secret crossed as %q — this response is the only place it can ever be read", got.Secret)
	}
	if !got.SecretChangedAt.Equal(changed) || got.PreviousSecretExpiresAt == nil || !got.PreviousSecretExpiresAt.Equal(expires) {
		t.Fatalf("the timestamps did not cross: %+v", got)
	}
}

// TestRotateResponseOmitsAnAbsentExpiry — the zero-window case on the wire. An
// expiry rendered as the zero time would read as "the old secret died in year
// zero", which is the kind of value that ends up in somebody's dashboard.
func TestRotateResponseOmitsAnAbsentExpiry(t *testing.T) {
	got := RotateClientSecretResponse{}.FromResult(commands.RotateClientSecretResult{
		ID:              domain.NewID("7b3c1f10-3c7e-4a8d-9f0e-9d2a8e6d4b51"),
		Secret:          "acs_x",
		SecretChangedAt: time.Date(2026, 8, 26, 14, 3, 11, 0, time.UTC),
	})
	if got.PreviousSecretExpiresAt != nil {
		t.Fatal("an absent expiry became a value")
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("the response did not encode: %v", err)
	}
	if json.Valid(body) && contains(string(body), "previousSecretExpiresAt") {
		t.Fatalf("the absent expiry is still on the wire: %s", body)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
