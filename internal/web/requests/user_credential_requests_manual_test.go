// Hand-written, and not a hook: no generator declares this file.
//
// The mappers for the two credential bodies. A field dropped in one of these is
// invisible in the worst way: the request parses, the route answers 204, and the
// value the caller sent never reached the aggregate — so a change that silently
// stopped carrying currentPassword would turn the proof of possession into a
// no-op while every other test stayed green.

package requests

import "testing"

func TestChangePasswordRequest_CarriesEveryField(t *testing.T) {
	r := ChangePasswordRequest{
		CurrentPassword:      "Str0ng!Passphrase",
		Password:             "An0ther!Passphrase",
		PasswordConfirmation: "An0ther!Passphrase",
	}

	cmd := r.ToCommand()

	if cmd.CurrentPassword != "Str0ng!Passphrase" {
		t.Errorf("the current password did not survive the mapper (%q) — the change would prove nothing", cmd.CurrentPassword)
	}
	if cmd.Password != "An0ther!Passphrase" {
		t.Errorf("the new password did not survive the mapper (%q)", cmd.Password)
	}
	if cmd.PasswordConfirmation != "An0ther!Passphrase" {
		t.Errorf("the confirmation did not survive the mapper (%q)", cmd.PasswordConfirmation)
	}
}

// TestResetPasswordRequest_CarriesNoCurrentPassword is as much about what the
// type does NOT have as about what it carries: the reset's whole premise is that
// the caller does not know the current password, and a body that grew one would
// be the change endpoint wearing the wrong URL.
func TestResetPasswordRequest_CarriesNoCurrentPassword(t *testing.T) {
	r := ResetPasswordRequest{
		Password:             "An0ther!Passphrase",
		PasswordConfirmation: "An0ther!Passphrase",
	}

	cmd := r.ToCommand()

	if cmd.Password != "An0ther!Passphrase" {
		t.Errorf("the new password did not survive the mapper (%q)", cmd.Password)
	}
	if cmd.PasswordConfirmation != "An0ther!Passphrase" {
		t.Errorf("the confirmation did not survive the mapper (%q)", cmd.PasswordConfirmation)
	}
}
