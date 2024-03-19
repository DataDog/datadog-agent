package service

import "github.com/DataDog/datadog-agent/pkg/util/log"

const (
	updaterUnit    = "datadog-updater.service"
	updaterUnitExp = "datadog-updater-exp.service"
)

var updaterUnits = []string{updaterUnit, updaterUnitExp}

func SetupUpdaterUnit() (err error) {
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup updater units: %s, reverting", err)
		}
	}()

	for _, unit := range updaterUnits {
		if err = loadUnit(unit); err != nil {
			return err
		}
	}

	if err = systemdReload(); err != nil {
		return err
	}

	// Should we kill ourselves after that? Otherwise I believe the systemd spawned
	// updater won't be able to bind to the sockets it needs if we are still alive.
	if err = startUnit(updaterUnit); err != nil {
		return err
	}
	return nil
}

func RemoveUpdaterUnit() {
	var err error
	for _, unit := range updaterUnits {
		if err = disableUnit(unit); err != nil {
			log.Warnf("Failed to disable %s: %s", unit, err)
		}
		if err = removeUnit(unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
		}
	}
}

func StartUpdaterExperiment() error {
	return startUnit(updaterUnitExp)
}

func StopUpdaterExperiment() error {
	return startUnit(updaterUnit)
}
