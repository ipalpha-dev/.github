package cli

import (
	"github.com/ipalpha-dev/tooling/internal/env"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

// askSeed asks "Seed with data?" once per workspace (Enter = yes; --yes = yes; --no-seed = no), stores the answer as
// seed=on|off in .ipalpha/settings and rewrites the tool-owned SEED_* keys of every .env.
func askSeed(w *workspace.Workspace, yes, noSeed bool) error {
	s := w.Settings
	switch {
	case noSeed:
		s.SetSeed(false)
	case s.SeedAsked():
		// Already chosen in an earlier setup: kept (change it with `ipalpha config set seed on|off` or --no-seed).
	case yes || !ui.Interactive():
		s.SetSeed(true)
	default:
		on, err := ui.Confirm(i18n.T("setup_seed"), i18n.T("setup_seed_desc"), true)
		if err != nil {
			return err
		}
		s.SetSeed(on)
	}
	if err := w.Save(); err != nil {
		return err
	}
	if _, err := env.Prepare(w); err != nil {
		return ui.Wrap(i18n.T("setup_env"), err)
	}
	if !s.SeedOn() {
		ui.Info(i18n.T("setup_seed_off"))
		return nil
	}
	ui.Done(i18n.T("setup_seed_on"))
	ui.Info(i18n.T("setup_seed_phones"))
	return nil
}

// featureSeedChoice is the seed choice a new feature workspace starts with: --no-seed means off; else main's value;
// when main never chose, ask once (Enter = yes) and remember it in main too. Returns "on" or "off".
func featureSeedChoice(w *workspace.Workspace, yes, noSeed bool) (string, error) {
	if noSeed {
		return "off", nil
	}
	mw, err := workspace.Open(w.Main())
	if err != nil {
		return "", err
	}
	if !mw.Settings.SeedAsked() {
		on := true
		if !yes && ui.Interactive() {
			if on, err = ui.Confirm(i18n.T("setup_seed"), i18n.T("setup_seed_desc"), true); err != nil {
				return "", err
			}
		}
		mw.Settings.SetSeed(on)
		if err := mw.Save(); err != nil {
			return "", err
		}
	}
	if mw.Settings.SeedOn() {
		return "on", nil
	}
	return "off", nil
}
