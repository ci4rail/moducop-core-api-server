<!--
SPDX-FileCopyrightText: 2026 Ci4Rail GmbH

SPDX-License-Identifier: Apache-2.0
-->

```mermaid
stateDiagram-v2
    [*] --> NeverDeployed: newEntity()

    NeverDeployed --> InProgress: update requested and mender idle
    NeverDeployed --> Waiting: update requested and mender busy

    Waiting --> InProgress: previous mender job finishes\nstart waiting update
    Waiting --> Failure: startEntityUpdate() fails when dequeued

    InProgress --> Success: mender job finished success\nand deployed version matches expected
    InProgress --> Failure: mender job finished failure
    InProgress --> Failure: mender job finished success\nbut deployed version mismatch

    Success --> InProgress: new update requested and mender idle
    Success --> Waiting: new update requested and mender busy

    Failure --> InProgress: new update requested and mender idle
    Failure --> Waiting: new update requested and mender busy

    note right of NeverDeployed
      "AlreadyDeployed" exists in the enum
      but is not assigned to entity state.
      That case is returned as an API error
      before any state transition.
    end note
```