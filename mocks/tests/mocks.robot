# EXECDIR must be mocks/

*** Settings ***
Library    Process
Library    OperatingSystem
Suite Setup    Setup Environment
Suite Teardown    Clear Environment

*** Variables ***
${STATE_DIR}    ${EXECDIR}/tests/mock-mender-state
${ASSET_DIR}    ${EXECDIR}/tests/assets
${VIRT_FS}      ${STATE_DIR}/fs

*** Test Cases ***

Initial Rootfs Update Shall Pass
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/Moducop-CPU01_Standard-Image_v2.6.0.f457f6d.20260210.1540.mender
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Installed, but not committed
    ${content}=    Get File        ${VIRT_FS}/etc/issue
        # not yet commited, so should still be the old rootfs
    Should Contain    ${content}    Ubuntu 24.04.1 LTS  

Rootfs Update Shall be Activated after Reboot
    ${result}=    Run Process   reboot
    Should Be Equal As Integers    ${result.rc}    0
    ${content}=    Get File        ${VIRT_FS}/etc/issue
    Should Contain    ${content}    Moducop-CPU01_Standard-Image_v2.6.0
    # Commit the update
    ${result}=    Run Process   mender-update  commit
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}   Committed.

Update shall be refused if update in progress
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/Moducop-CPU01_Standard-Image_v2.7.0.40ee657.20260218.1208.mender
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Installed, but not committed
    # try to install again without rebooting first, should be refused
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/Moducop-CPU01_Standard-Image_v2.6.0.f457f6d.20260210.1540.mender
    Should Be Equal As Integers    ${result.rc}    1
    Should Contain    ${result.stdout}   Update already in progress
    # try to install app update, should also be refused
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
    Should Be Equal As Integers    ${result.rc}    1
    Should Contain    ${result.stdout}   Update already in progress

Commit Rootfs Without Reboot Shall Cause Rollback
    ${result}=    Run Process   mender-update  commit
    Should Be Equal As Integers    ${result.rc}    1
    Should Contain    ${result.stdout}   Installation failed. Rolled back modifications.
    ${content}=    Get File        ${VIRT_FS}/etc/issue
    Should Contain    ${content}    Moducop-CPU01_Standard-Image_v2.6.0

Update for different Machine Shall be Refused
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/Moducop-CPU01Plus_Standard-Image_v2.7.0.40ee657.20260218.1033.mender
    Should Be Equal As Integers    ${result.rc}    1
    Should Contain    ${result.stdout}    Artifact device type doesn't match

Initial Application Update Shall Pass
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
    # Log To Console    ${result.stderr}
    # Log To Console    ${result.stdout}

    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Installed and committed
    
    ${result}=  Run Docker PS WithLabels
    Should Contain  ${result.stdout}    nginx-demo-web-1
    Should Contain  ${result.stdout}    com.docker.compose.project=nginx-demo
    Should Contain  ${result.stdout}    software-version=8f249b9

New Application Update Shall Pass
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-a895c3c.mender
    Log To Console    ${result.stderr}
    Log To Console    ${result.stdout}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Installed and committed
    
    ${result}=  Run Docker PS WithLabels
    Should Contain  ${result.stdout}    nginx-demo-web-1
    Should Contain  ${result.stdout}    com.docker.compose.project=nginx-demo
    Should Contain  ${result.stdout}    com.ci4rail.app.software-version=a895c3c
    
Application Error Inject After Stop Old Containers
    # ensure there is an existing deployed app that can become inconsistent
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-a895c3c.mender
    Should Be Equal As Integers    ${result.rc}    0

    ${result}=    Run Process   mender-update   err-inject    after-stop-old-containers
    Should Be Equal As Integers    ${result.rc}    0

    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
    Clear Error Injection

    # no more containers should be running
    ${result}=  Run Docker PS WithLabels
    Should Not Contain  ${result.stdout}    nginx-demo-web-1
    Should Not Contain  ${result.stdout}    com.docker.compose.project=nginx-demo

    # try to install again, should be refused
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
    Should Be Equal As Integers    ${result.rc}    1
    Log To Console   stdout: ${result.stdout}
    Should Contain    ${result.stdout}   Update already in progress

    # To get out of this situation, we need to commit the update, so that I can try again installation
    ${result}=    Run Process   mender-update  commit
    Should Be Equal As Integers    ${result.rc}    0
    Log To Console  stdout: ${result.stdout}
    Should Contain    ${result.stdout}    Installation failed, and Update Module does not support rollback. System may be in an inconsistent state.

    # next install is blocked until stale app dirs are removed manually
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
    Should Be Equal As Integers    ${result.rc}    1
    Should Contain    ${result.stdout}    Installation failed, and Update Module does not support rollback. System may be in an inconsistent state.

    Run Keyword And Ignore Error    Remove Directory    ${VIRT_FS}/data/mender-app/nginx-demo    recursive=True
    Run Keyword And Ignore Error    Remove Directory    ${VIRT_FS}/data/mender-app/nginx-demo-previous    recursive=True

    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Installed and committed.


*** Keywords ***
Setup Environment
    ${path}=    Get Environment Variable    PATH
    ${newpath}=    Set Variable    ${EXECDIR}/bin:${path}
    Set Environment Variable    PATH    ${newpath}
    Remove Directory    ${STATE_DIR}    recursive=True
    Set Environment Variable    MOCK_MENDER_STATE_DIR     ${STATE_DIR}

Clear Environment
    Clear Error Injection

Run Docker PS WithLabels
    ${result}=    Run Process    docker  ps   --format  {{.Names}}\\t{{.Labels}}
    Log To Console    ${result.stderr}
    Log To Console    ${result.stdout}

    RETURN    ${result}

Clear Error Injection
    ${result}=    Run Process   mender-update   err-inject   \
    Should Be Equal As Integers    ${result.rc}    0
