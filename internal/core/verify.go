package core

import (
	"fmt"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/github"
)

// The checksum rules. With a checksum: verified, installed without asking. Without one: asks, unless
// the app is trusted (AllowUnverified). Without one when earlier releases had one: always asks.

// offer records an asset as the available update and reports whether to go on and install it: not
// when only checking (DryRun).
func (e *Engine) offer(app *applist.App, asset *github.Asset, r *Result) bool {
	if asset.Digest != "" {
		app.HadDigest = true
	}
	r.set(UpdateAvailable, verificationNote(app, asset))
	return !e.DryRun
}

// approved applies the checksum rules, asking when they say so; a "no" is recorded on r.
func (e *Engine) approved(app *applist.App, asset *github.Asset, r *Result) bool {
	if e.approve(app, asset) {
		return true
	}
	r.set(Skipped, "Unverified download declined")
	return false
}

func verificationNote(app *applist.App, asset *github.Asset) string {
	switch {
	case asset.Digest != "":
		return "Checksum available"
	case app.HadDigest:
		return "No checksum (unusual: earlier releases had one)"
	case app.AllowUnverified:
		return "No checksum (trusted app)"
	}
	return "No checksum (will ask before installing)"
}

// verifiedNote says how a finished download was checked.
func verifiedNote(asset *github.Asset) string {
	if asset.Digest != "" {
		return "Checksum verified"
	}
	return "Unverified"
}

func (e *Engine) approve(app *applist.App, asset *github.Asset) bool {
	switch {
	case asset.Digest != "":
		return true
	case app.HadDigest:
		e.warn("%s: earlier releases had a checksum, but %s has none. That is unusual; check the release page before installing.", app.Name, asset.Name)
		return e.yes(fmt.Sprintf("Install %s anyway?", asset.Name))
	case app.AllowUnverified:
		e.verbose("No checksum for %s; installing because %s is trusted.", asset.Name, app.Name)
		return true
	}
	switch e.Ask.Choose(fmt.Sprintf("%s has no checksum, so it can't be verified. Install anyway? (Always = trust this app from now on)", asset.Name),
		[]string{"Yes", "Always", "No"}, 2) {
	case 0:
		return true
	case 1:
		app.AllowUnverified = true
		return true
	}
	return false
}

// checkSigner checks an installer's digital signature and remembers who signed the app. A broken
// signature (a file changed after it was signed) installs nothing. A release signed by someone else
// than earlier ones, or not signed when they were, is unusual (a hacked account can upload a release
// with a valid checksum, but not with the developer's signature), so it asks first. Apps that were
// never signed are installed as before.
func (e *Engine) checkSigner(app *applist.App, asset *github.Asset, file string) (signer string, ok bool, err error) {
	if signer, err = e.System.Signer(file); err != nil {
		return "", false, fmt.Errorf("%s: %v. Nothing was installed", asset.Name, err)
	}
	switch {
	case signer == app.Publisher:
	case app.Publisher == "":
		app.Publisher = signer
	default:
		now := "isn't signed"
		if signer != "" {
			now = "is signed by " + signer
		}
		e.warn("%s: earlier releases were signed by %s, but %s %s. That is unusual; check the release page before installing.", app.Name, app.Publisher, asset.Name, now)
		if !e.yes(fmt.Sprintf("Install %s anyway?", asset.Name)) {
			return signer, false, nil
		}
		if signer != "" {
			app.Publisher = signer
		}
	}
	return signer, true, nil
}

// yes asks a yes/no question whose safe answer, and default, is No.
func (e *Engine) yes(question string) bool {
	return e.Ask.Choose(question, []string{"Yes", "No"}, 1) == 0
}
