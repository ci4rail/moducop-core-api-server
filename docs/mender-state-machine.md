<!--
SPDX-FileCopyrightText: 2026 Ci4Rail GmbH

SPDX-License-Identifier: Apache-2.0
-->

```mermaid
stateDiagram-v2
    [*] --> Idle

    Idle --> Installing: StartUpdateJob / state idle

    Installing --> Installing: Restarted\nrestart install
    Installing --> Installing: RecoverFinished(success)\nrestart install
    Installing --> Rebooting: InstallFinished(InstalledButNotCommitted)\nCoreOS update
    Installing --> Committing: InstallFinished(InstalledButNotCommitted)\nApplication update
    Installing --> Idle: InstallFinished(InstalledAndCommitted or Committed)\nemit JobFinished(success)
    Installing --> RecoverInstallCommitting: InstallFinished(PleaseCommitOrRollback or UpdateAlreadyInProgress)\nstart recovery install
    Installing --> RecoverInstallClearApp: InstallFinished(SystemInconsistent)\napplication update
    Installing --> Idle: InstallFinished(SystemInconsistent)\nCoreOS update, emit JobFinished(failure)
    Installing --> Idle: InstallFinished(SystemNotModified or RolledBack or Generic)\nemit JobFinished(failure)

    Rebooting --> Rebooting: Restarted\nretry reboot
    Rebooting --> Committing: RebootFinished(success)\nrun commit
    Rebooting --> Idle: RebootFinished(failure)\nemit JobFinished(failure)

    Committing --> Committing: Restarted\nretry commit
    Committing --> Idle: CommitFinished(success)\nemit JobFinished(success)
    Committing --> Idle: CommitFinished(failure)\nemit JobFinished(failure)

    RecoverInstallCommitting --> RecoverInstallCommitting: Restarted\nstart recovery install
    RecoverInstallCommitting --> Installing: CommitFinished(InstalledAndCommitted or Committed)\nemit RecoverFinished(success)
    RecoverInstallCommitting --> RecoverInstallCommitting: CommitFinished(PleaseCommitOrRollback)\nstart recovery install again
    RecoverInstallCommitting --> RecoverInstallClearApp: CommitFinished(SystemInconsistent)\napplication update
    RecoverInstallCommitting --> Idle: CommitFinished(SystemInconsistent)\nCoreOS update, emit JobFinished(failure)
    RecoverInstallCommitting --> Idle: CommitFinished(SystemNotModified or RolledBack or UpdateAlreadyInProgress or InstalledButNotCommitted or Generic)\nemit RecoverFinished(failure) -> restart install state machine emits JobFinished(failure)

    RecoverInstallClearApp --> RecoverInstallClearApp: Restarted\nclear app dir again
    RecoverInstallClearApp --> Installing: clearAppDir complete\nemit RecoverFinished(success)

    note right of Idle
      JobFinished always calls setIdle()
      before emitting the event.
    end note
```