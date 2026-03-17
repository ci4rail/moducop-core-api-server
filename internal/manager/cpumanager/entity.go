package cpumanager

import (
	"errors"
	"fmt"

	"github.com/ci4rail/moducop-core-api-server/internal/menderartifact"
)

type entityType int

const (
	entityTypeCoreOs entityType = iota
	entityTypeApplication
)

type entity struct {
	Name           string
	EntityType     entityType
	DeployStatus   DeployStatus
	MenderArtifact string      // "" if no update is in progress
	DeployingNV    NameVersion // the version that is being deployed, empty if no deployment in progress
}

var errUnknownEntityType = errors.New("unknown entity type")

func newEntity(name string, entityType entityType) *entity {
	e := &entity{
		Name:           name,
		DeployStatus:   DeployStatus{Code: DeployStatusCodeNeverDeployed, Message: "Entity has never been deployed"},
		MenderArtifact: "",
	}

	switch entityType {
	case entityTypeCoreOs:
		e.EntityType = entityTypeCoreOs
	case entityTypeApplication:
		e.EntityType = entityTypeApplication
	default:
		return nil
	}

	return e
}

// getEntityInProgress returns the entity that is currently being updated (DeployStatusCodeInProgress)
// or nil if there is no such entity.
// By design, only one entity can be in progress at a time, so this function returns as soon as it finds one.
func (m *CPUManager) getEntityInProgress() *entity {
	for _, e := range m.state.Entities {
		if e.DeployStatus.Code == DeployStatusCodeInProgress {
			return e
		}
	}
	return nil
}

// getEntityWaiting gets the next waiting entity or nil if there is no waiting entity. If there are multiple waiting entities,
// it returns the first one found in the map iteration order (which is random).
func (m *CPUManager) getEntityWaiting() *entity {
	for _, e := range m.state.Entities {
		if e.DeployStatus.Code == DeployStatusCodeWaiting {
			return e
		}
	}
	return nil
}

func (m *CPUManager) startEntityUpdate(e *entity) error {
	e.DeployStatus.Code = DeployStatusCodeInProgress
	e.DeployStatus.Message = "Update is in progress"
	err := m.mender.StartUpdateJob(e.EntityType, e.MenderArtifact, e.Name, updateTimeout)
	if err != nil {
		e.DeployStatus.Code = DeployStatusCodeFailure
		e.DeployStatus.Message = fmt.Sprintf("Failed to start update: %v", err)
		if errors.Is(err, ErrMenderBusy) {
			return WrapCodedError(
				ErrCodeMenderBusy,
				fmt.Sprintf("mender is busy, cannot start update for entity %s", e.Name),
				err,
			)
		}
		return WrapCodedError(
			ErrCodeStartUpdateFailed,
			fmt.Sprintf("failed to start mender update job for entity %s", e.Name),
			err,
		)
	}
	return nil
}

func (m *CPUManager) finishEntityUpdate(e *entity, success bool, message string) {
	if success {
		deployed, err := e.isDeployed(e.DeployingNV)
		if err != nil {
			m.logger.Errorf("Failed to check if entity %s is deployed: %v", e.Name, err)
			deployed = false
		}
		if deployed {
			e.DeployStatus.Code = DeployStatusCodeSuccess
			e.DeployStatus.Message = "Update deployed successfully"
		} else {
			e.DeployStatus.Code = DeployStatusCodeFailure
			e.DeployStatus.Message = fmt.Sprintf("Mender reported success, but deployed version does not match expected version %s", e.DeployingNV)
		}
	} else {
		e.DeployStatus.Code = DeployStatusCodeFailure
		e.DeployStatus.Message = message
	}
	e.MenderArtifact = ""
	e.DeployingNV = NameVersion{}
}

// getDeployedVersion returns the currently deployed version for the given entity.
// Returns nameVersion, error. "nameVersion.name" for applications is the same as the entity name
func (e *entity) getDeployedVersion() (NameVersion, error) {
	switch e.EntityType {
	case entityTypeCoreOs:
		name, version, err := coreOSVersionFromTargetFS()
		if err != nil {
			return NameVersion{}, fmt.Errorf("get version for CoreOS: %w", err)
		}
		return NameVersion{Name: name, Version: version}, nil
	case entityTypeApplication:
		version, err := appVersionFromTargetFS(e.Name)
		if err != nil {
			return NameVersion{}, fmt.Errorf("get version for application %s: %w", e.Name, err)
		}
		return NameVersion{Name: e.Name, Version: version}, nil
	default:
		return NameVersion{}, fmt.Errorf("%w: %v", errUnknownEntityType, e.EntityType)
	}
}

// getVersionFromArtifact extracts the version information from the given artifact for the given entity.
// Returns nameVersion, error. "nameVersion.name" for applications is the same as the entity name
func (e *entity) getVersionFromArtifact(artifact string) (NameVersion, error) {
	switch e.EntityType {
	case entityTypeCoreOs:
		name, version, err := menderartifact.CoreOSVersionFromArtifact(artifact)
		if err != nil {
			return NameVersion{}, fmt.Errorf("get version from artifact for CoreOS: %w", err)
		}
		return NameVersion{Name: name, Version: version}, nil
	case entityTypeApplication:
		version, err := menderartifact.AppVersionFromArtifact(artifact, e.Name)
		if err != nil {
			return NameVersion{}, fmt.Errorf("get version from artifact for application %s: %w", e.Name, err)
		}
		return NameVersion{Name: e.Name, Version: version}, nil
	default:
		return NameVersion{}, fmt.Errorf("%w: %v", errUnknownEntityType, e.EntityType)
	}
}

// isDeployed checks if the given artifact is deployed for this entity
// by comparing the version from the artifact with the currently deployed version.
func (e *entity) isDeployed(expected NameVersion) (bool, error) {
	deployedNV, err := e.getDeployedVersion()
	if err != nil {
		return false, fmt.Errorf("get deployed version: %w", err)
	}
	return deployedNV.Name == expected.Name && deployedNV.Version == expected.Version, nil
}
