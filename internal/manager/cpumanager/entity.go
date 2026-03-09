package cpumanager

import (
	"errors"
	"fmt"
)

type entityType int

const (
	entityTypeCoreOs entityType = iota
	entityTypeApplication
)

type entity struct {
	Name         string
	EntityType   entityType
	DeployStatus DeployStatus
	MenderFile   string // "" if no update is in progress
}

func newEntity(name string, entityType entityType) *entity {
	e := &entity{
		Name:         name,
		DeployStatus: DeployStatus{Code: DeployStatusCodeNeverDeployed, Message: "Entity has never been deployed"},
		MenderFile:   "",
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

func (m *CPUManager) getEntityInProgress() *entity {
	for _, e := range m.state.Entities {
		if e.DeployStatus.Code == DeployStatusCodeInProgress {
			return e
		}
	}
	return nil
}

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
	err := m.mender.StartUpdateJob(e.EntityType, e.MenderFile, updateStartTimeout)
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
		e.DeployStatus.Code = DeployStatusCodeSuccess
		e.DeployStatus.Message = "Update deployed successfully"
	} else {
		e.DeployStatus.Code = DeployStatusCodeFailure
		e.DeployStatus.Message = message
	}
	e.MenderFile = ""
}

func (e *entity) getDeployedVersion() string {
	// TODO:
	return ""
}
