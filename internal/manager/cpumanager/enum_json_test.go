package cpumanager

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPersistentStateMarshalsEnumsAsStrings(t *testing.T) {
	t.Parallel()

	state := persistentState{
		MenderState: menderPersistentState{
			State:             menderStateInstalling,
			CurrentArtifact:   "/tmp/update.mender",
			CurrentEntityType: entityTypeApplication,
		},
		Entities: map[string]*entity{
			"coreos": {
				Name:       "coreos",
				EntityType: entityTypeCoreOs,
				DeployStatus: DeployStatus{
					Code:    DeployStatusCodeInProgress,
					Message: "running",
				},
				MenderArtifact: "/tmp/core.mender",
			},
		},
	}

	out, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	s := string(out)
	if !strings.Contains(s, `"State":"installing"`) {
		t.Fatalf("expected mender state as string, got: %s", s)
	}
	if !strings.Contains(s, `"CurrentEntityType":"application"`) {
		t.Fatalf("expected current entity type as string, got: %s", s)
	}
	if !strings.Contains(s, `"EntityType":"coreos"`) {
		t.Fatalf("expected entity type as string, got: %s", s)
	}
	if !strings.Contains(s, `"code":"in_progress"`) {
		t.Fatalf("expected deploy status code as string, got: %s", s)
	}
}

func TestPersistentStateUnmarshalRejectsNumericEnums(t *testing.T) {
	t.Parallel()

	legacy := `{
		"MenderState": {
			"State": 2,
			"CurrentFile": "",
			"CurrentEntityType": 1
		},
		"Entities": {
			"coreos": {
				"Name": "coreos",
				"EntityType": 0,
				"DeployStatus": {
					"Code": 4,
					"Message": "done"
				},
				"MenderFile": ""
			}
		}
	}`

	var state persistentState
	if err := json.Unmarshal([]byte(legacy), &state); err == nil {
		t.Fatalf("Unmarshal() error = nil, want error")
	}
}
