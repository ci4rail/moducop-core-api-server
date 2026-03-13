package cpumanager

import (
	"encoding/json"
	"errors"
	"fmt"
)

var (
	errInvalidEntityTypeValue      = errors.New("invalid entity type value")
	errInvalidDeployStatusCode     = errors.New("invalid deploy status code")
	errInvalidMenderState          = errors.New("invalid mender state")
	errUnsupportedEnumJSONEncoding = errors.New("unsupported enum JSON encoding")
)

func (e entityType) String() string {
	switch e {
	case entityTypeCoreOs:
		return "coreos"
	case entityTypeApplication:
		return "application"
	default:
		return ""
	}
}

func (e entityType) MarshalJSON() ([]byte, error) {
	s := e.String()
	if s == "" {
		return nil, fmt.Errorf("%w: %d", errInvalidEntityTypeValue, e)
	}
	return json.Marshal(s)
}

func (e *entityType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("%w: %s", errUnsupportedEnumJSONEncoding, string(data))
	}
	switch s {
	case "coreos":
		*e = entityTypeCoreOs
	case "application":
		*e = entityTypeApplication
	default:
		return fmt.Errorf("%w: %q", errInvalidEntityTypeValue, s)
	}
	return nil
}

func (d DeployStatusCode) String() string {
	switch d {
	case DeployStatusCodeNeverDeployed:
		return "never_deployed"
	case DeployStatusCodeWaiting:
		return "waiting"
	case DeployStatusCodeAlreadyDeployed:
		return "already_deployed"
	case DeployStatusCodeInProgress:
		return "in_progress"
	case DeployStatusCodeSuccess:
		return "success"
	case DeployStatusCodeFailure:
		return "failure"
	default:
		return ""
	}
}

func (d DeployStatusCode) MarshalJSON() ([]byte, error) {
	s := d.String()
	if s == "" {
		return nil, fmt.Errorf("%w: %d", errInvalidDeployStatusCode, d)
	}
	return json.Marshal(s)
}

func (d *DeployStatusCode) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("%w: %s", errUnsupportedEnumJSONEncoding, string(data))
	}
	parsed, ok := parseDeployStatusCodeText(s)
	if !ok {
		return fmt.Errorf("%w: %q", errInvalidDeployStatusCode, s)
	}
	*d = parsed
	return nil
}

func parseDeployStatusCodeText(value string) (DeployStatusCode, bool) {
	switch value {
	case "never_deployed":
		return DeployStatusCodeNeverDeployed, true
	case "waiting":
		return DeployStatusCodeWaiting, true
	case "already_deployed":
		return DeployStatusCodeAlreadyDeployed, true
	case "in_progress":
		return DeployStatusCodeInProgress, true
	case "success":
		return DeployStatusCodeSuccess, true
	case "failure":
		return DeployStatusCodeFailure, true
	default:
		return 0, false
	}
}

func (s menderState) String() string {
	switch s {
	case menderStateIdle:
		return "idle"
	case menderStateInstalling:
		return "installing"
	case menderStateRebooting:
		return "rebooting"
	case menderStateCommitting:
		return "committing"
	case menderStateRecoverInstall:
		return "recover_install"
	default:
		return ""
	}
}

func (s menderState) MarshalJSON() ([]byte, error) {
	text := s.String()
	if text == "" {
		return nil, fmt.Errorf("%w: %d", errInvalidMenderState, s)
	}
	return json.Marshal(text)
}

func (s *menderState) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("%w: %s", errUnsupportedEnumJSONEncoding, string(data))
	}
	switch text {
	case "idle":
		*s = menderStateIdle
	case "installing":
		*s = menderStateInstalling
	case "rebooting":
		*s = menderStateRebooting
	case "committing":
		*s = menderStateCommitting
	case "recover_install":
		*s = menderStateRecoverInstall
	default:
		return fmt.Errorf("%w: %q", errInvalidMenderState, text)
	}
	return nil
}
