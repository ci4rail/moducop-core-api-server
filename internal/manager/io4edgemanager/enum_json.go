package io4edgemanager

import (
	"encoding/json"
	"errors"
	"fmt"
)

var (
	errInvalidDeployStatusCode     = errors.New("invalid deploy status code")
	errUnsupportedEnumJSONEncoding = errors.New("unsupported enum JSON encoding")
)

func (d DeployStatusCode) String() string {
	switch d {
	case DeployStatusCodeNeverDeployed:
		return "never_deployed"
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
