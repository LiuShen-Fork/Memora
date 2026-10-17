package main

import (
	"fmt"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

// minPasswordLength is the shortest password this site accepts, wherever a
// password is set: the setup wizard, logging in, and changing one's own.
const minPasswordLength = 6

// changePassword sets a password of the person's own on the account they are
// signed in with. The password in use is asked for again, so that a session
// someone else has got hold of is not enough to take the account over.
//
// This is also the way off the password ChronoFrame used to create accounts
// with: the setup wizard refuses it for new sites, and an account that already
// has it can be given a new one here.
func (a *App) changePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := a.require(r)
	if !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var body struct{ CurrentPassword, NewPassword string }
	if decodeJSON(r, &body) != nil {
		errorJSON(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if len(body.NewPassword) < minPasswordLength {
		errorJSON(w, http.StatusBadRequest, fmt.Sprintf("Passwords need at least %d characters", minPasswordLength))
		return
	}
	if problem := setupPasswordProblem(body.NewPassword); problem != "" {
		errorJSON(w, http.StatusBadRequest, problem)
		return
	}

	id, _ := user["id"].(int64)
	var hash string
	if a.db.QueryRow(`SELECT COALESCE(password,'') FROM users WHERE id=?`, id).Scan(&hash) != nil {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	// An account created by signing in with GitHub has no password yet. The
	// session it is making this request with is the only thing there is to
	// check, so a first password can simply be set; afterwards the password
	// that was chosen has to be given again like anywhere else.
	if hash != "" && !verifyPassword(hash, body.CurrentPassword) {
		errorJSON(w, http.StatusForbidden, "That is not your current password")
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to change the password")
		return
	}
	if _, err := a.db.Exec(`UPDATE users SET password=? WHERE id=?`, string(hashed), id); err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to change the password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
