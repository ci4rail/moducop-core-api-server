package cpumanager

import "context"

type Result[T any] struct {
	Value T
	Err   error
}

type command interface {
	isCommand()
}

type DeployStatusCode int

const (
	DeployStatusCodeNeverDeployed DeployStatusCode = iota
	DeployStatusCodeWaiting                        // for mender to become available for update
	DeployStatusCodeAlreadyDeployed
	DeployStatusCodeInProgress
	DeployStatusCodeSuccess
	DeployStatusCodeFailure
)

type DeployStatus struct {
	Code    DeployStatusCode
	Message string
}

type EntityStatus struct {
	DeployStatus   DeployStatus
	CurrentVersion string
}

type StartCoreOsUpdate struct {
	// path to the mender file to be installed
	PathToMenderFile string
	// channel where the result of the command will be sent back
	Reply chan Result[struct{}]
}

func (StartCoreOsUpdate) isCommand() {}

type GetCoreOsState struct {
	Reply chan Result[EntityStatus]
}

func (GetCoreOsState) isCommand() {}

type StartApplicationUpdate struct {
	AppName string
	// path to the mender file to be installed
	PathToMenderFile string
	// channel where the result of the command will be sent back
	Reply chan Result[struct{}]
}

func (StartApplicationUpdate) isCommand() {}

type GetApplicationState struct {
	AppName string
	Reply   chan Result[EntityStatus]
}

func (GetApplicationState) isCommand() {}

type Reboot struct {
	Reply chan Result[struct{}]
}

func (Reboot) isCommand() {}

// internal commands can be defined here, e.g. for handling events
type MenderEvent struct {
	event menderEvent
}

func (MenderEvent) isCommand() {}

// Ask sends a command to the manager's inbox and waits for the result on the reply channel. It returns an error
// if the context is done before the command is sent or before the result is received.
func Ask[T any](ctx context.Context, m *CPUManager, cmd command, reply <-chan Result[T]) (T, error) {
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
