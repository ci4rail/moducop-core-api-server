package io4edgemanager

import "context"

type Result[T any] struct {
	Value T
	Err   error
}

type Command interface {
	isCommand()
}

type DeployStatusCode int

const (
	DeployStatusCodeNeverDeployed DeployStatusCode = iota
	DeployStatusCodeAlreadyDeployed
	DeployStatusCodeInProgress
	DeployStatusCodeSuccess
	DeployStatusCodeFailure
)

type DeployStatus struct {
	Code    DeployStatusCode `json:"code"`
	Message string           `json:"message"`
}

type NameVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Io4edgeFWStatus struct {
	DeployStatus DeployStatus `json:"deploy_status"`
	Current      NameVersion  `json:"current"`
}

type StartUpdate struct {
	// Device Name to be updated
	DeviceName string
	// path to firmware package to be installed
	PathToFWPKG string
	// channel where the result of the command will be sent back
	Reply chan Result[struct{}]
}

func (StartUpdate) isCommand() {}

type GetState struct {
	// Device Name to get the state for
	DeviceName string
	// channel where the result of the command will be sent back
	Reply chan Result[Io4edgeFWStatus]
}

func (GetState) isCommand() {}

type ListDeviceNames struct {
	Reply chan Result[[]string]
}

func (ListDeviceNames) isCommand() {}

// internal commands can be defined here, e.g. for handling events
type cliEvent struct {
	Success bool
	Message string
}

func (cliEvent) isCommand() {}

// Ask sends a command to the manager's inbox and waits for the result on the reply channel. It returns an error
// if the context is done before the command is sent or before the result is received.
func Ask[T any](ctx context.Context, m *Io4edgeManager, cmd Command, reply <-chan Result[T]) (T, error) {
	var zero T

	select {
	case m.inbox <- cmd:
	case <-ctx.Done():
		return zero, ctx.Err()
	}

	select {
	case res := <-reply:
		return res.Value, res.Err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}
