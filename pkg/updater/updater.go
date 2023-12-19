package updater

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/DataDog/datadog-agent/pkg/updater/packaging"
)

// Updater is the updater used to update packages.
type Updater struct {
	repository *packaging.Repository
	downloader *packaging.Downloader
}

// NewUpdater returns a new Updater.
func NewUpdater(repositoryPath string) *Updater {
	return &Updater{
		repository: &packaging.Repository{RootPath: repositoryPath},
		downloader: packaging.NewDownloader(http.DefaultClient),
	}
}

// Initialize initializes the updater with the given package.
// If the underlying repository already exists, it will be overwritten.
func (u *Updater) Initialize(ctx context.Context, firstPackage packaging.RemotePackage) error {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	err = u.downloader.Download(ctx, firstPackage, tmpDir)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	err = u.repository.Create(tmpDir)
	if err != nil {
		return fmt.Errorf("could not create repository: %w", err)
	}
	return nil
}

// StartExperiment starts an experiment with the given package.
func (u *Updater) StartExperiment(ctx context.Context, experimentPackage packaging.RemotePackage) error {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	err = u.downloader.Download(ctx, experimentPackage, tmpDir)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	err = u.repository.SetExperiment(tmpDir)
	if err != nil {
		return fmt.Errorf("could not set experiment: %w", err)
	}
	return nil
}

// PromoteExperiment promotes the experiment to stable.
func (u *Updater) PromoteExperiment() error {
	err := u.repository.PromoteExperiment()
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	return nil
}

// StopExperiment stops the experiment.
func (u *Updater) StopExperiment() error {
	err := u.repository.DeleteExperiment()
	if err != nil {
		return fmt.Errorf("could not set stable: %w", err)
	}
	return nil
}
