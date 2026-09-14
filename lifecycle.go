package svcforge

import "fmt"

// InstallDecision is the portable decision made before platform mutation.
type InstallDecision struct {
	Operation       Operation
	PreviousVersion string
	CurrentVersion  string
}

// DecideInstall determines whether --install means a fresh install, upgrade or rejection.
func DecideInstall(app App, installed *Manifest) (InstallDecision, error) {
	if err := app.Validate(); err != nil {
		return InstallDecision{}, err
	}
	if installed == nil {
		return InstallDecision{Operation: OperationInstall, CurrentVersion: app.Version}, nil
	}
	if installed.AppID != app.ID {
		return InstallDecision{}, fmt.Errorf("installed manifest belongs to %q, not %q", installed.AppID, app.ID)
	}
	decision := InstallDecision{
		Operation:       OperationUpgrade,
		PreviousVersion: installed.Version,
		CurrentVersion:  app.Version,
	}
	if app.Version == "" || installed.Version == "" {
		return decision, nil
	}
	comparison, err := CompareVersions(app.Version, installed.Version)
	if err != nil {
		return InstallDecision{}, err
	}
	switch {
	case comparison == 0:
		return InstallDecision{}, ErrSameVersion
	case comparison < 0 && !app.Upgrade.AllowDowngrade:
		return InstallDecision{}, fmt.Errorf("%w: installed=%s requested=%s", ErrDowngradeBlocked, installed.Version, app.Version)
	default:
		return decision, nil
	}
}
