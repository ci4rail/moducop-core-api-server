# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

# EXECDIR must be mocks/

*** Settings ***
Library    Process
Library    OperatingSystem
Suite Setup    Setup Environment
Suite Teardown    Clear Environment
Test Tags  mender

*** Variables ***
${STATE_DIR}    ${EXECDIR}/tests/mock-mender-state
${ASSET_DIR}    ${EXECDIR}/../tests/assets
${VIRT_FS}      ${STATE_DIR}/fs

*** Test Cases ***

BootID shall Exist
    ${content}=    Get File        ${VIRT_FS}/proc/sys/kernel/random/boot_id
    Should Not Be Empty    ${content}

Reboot shall change BootID
    ${content}=    Get File        ${VIRT_FS}/proc/sys/kernel/random/boot_id
    ${bootid_before}=    Set Variable    ${content}
    ${result}=    Run Process   reboot
    Should Be Equal As Integers    ${result.rc}    0
    ${content}=    Get File        ${VIRT_FS}/proc/sys/kernel/random/boot_id
    ${bootid_after}=    Set Variable    ${content}
    Should Not Be Equal   ${bootid_before}   ${bootid_after}


Initial Rootfs Update Shall Pass
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/Moducop-CPU01_Standard-Image_v2.7.0.40ee657.20260218.1208.mender
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Installed, but not committed
    ${content}=    Get File        ${VIRT_FS}/etc/issue
        # not yet commited, so should still be the old rootfs
    Should Contain    ${content}    Moducop-CPU01_Standard-Image_v2.6.0

Rootfs Update Shall be Activated after Reboot
    ${result}=    Run Process   reboot
    Should Be Equal As Integers    ${result.rc}    0
    ${content}=    Get File        ${VIRT_FS}/etc/issue
    Should Contain    ${content}    Moducop-CPU01_Standard-Image_v2.7.0
    # Commit the update
    ${result}=    Run Process   mender-update  commit
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}   Committed.

Update shall be refused if update in progress
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/Moducop-CPU01_Standard-Image_v2.6.0.f457f6d.20260210.1540.mender
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Installed, but not committed
    # try to install again without rebooting first, should be refused
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/Moducop-CPU01_Standard-Image_v2.7.0.40ee657.20260218.1208.mender
    Should Be Equal As Integers    ${result.rc}    1
    Should Contain    ${result.stdout}${result.stderr}   Update already in progress
    # try to install app update, should also be refused
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
    Should Be Equal As Integers    ${result.rc}    1
    Should Contain    ${result.stdout}${result.stderr}   Update already in progress

Commit Rootfs Without Reboot Shall Cause Rollback
    ${result}=    Run Process   mender-update  commit
    Should Be Equal As Integers    ${result.rc}    1
    Should Contain    ${result.stdout}   ${COMMIT_ROLLBACK_OUTPUT}
    ${content}=    Get File        ${VIRT_FS}/etc/issue
    Should Contain    ${content}    Moducop-CPU01_Standard-Image_v2.7.0

Update for different Machine Shall be Refused
    ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/Moducop-CPU01Plus_Standard-Image_v2.7.0.40ee657.20260218.1033.mender
    Should Be Equal As Integers    ${result.rc}    1
    Should Contain    ${result.stdout}${result.stderr}    Artifact device type doesn't match

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
    Should Contain    ${result.stdout}${result.stderr}   Update already in progress

    IF    '${MENDER_VERSION}' == '5'
        ${result}=    Run Process    mender-update    commit
        Should Be Equal As Integers    ${result.rc}    1
        Should Contain    ${result.stderr}    Cannot commit from this state.
        ${result}=    Run Process    mender-update    resume
        Should Be Equal As Integers    ${result.rc}    0
        Should Contain    ${result.stdout}    Installed and committed.
        ${result}=    Run Docker PS WithLabels
        Should Contain    ${result.stdout}    nginx-demo-web-1
    ELSE
        # To get out of this situation, we need to commit the update, so that I can try again installation
        ${result}=    Run Process   mender-update  commit
        Should Be Equal As Integers    ${result.rc}    0
        Log To Console  stdout: ${result.stdout}
        Should Contain    ${result.stdout}    ${INCONSISTENT_OUTPUT}

        # next install is blocked until stale app dirs are removed manually
        ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
        Should Be Equal As Integers    ${result.rc}    1
        Should Contain    ${result.stdout}    ${INCONSISTENT_OUTPUT}

        Run Keyword And Ignore Error    Remove Directory    ${VIRT_FS}/data/mender-app/nginx-demo    recursive=True
        Run Keyword And Ignore Error    Remove Directory    ${VIRT_FS}/data/mender-app/nginx-demo-previous    recursive=True

        ${result}=    Run Process   mender-update  install  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
        Should Be Equal As Integers    ${result.rc}    0
        Should Contain    ${result.stdout}    Installed and committed.
    END

Core OS Customization Status Shall Reflect Health Check Result
    ${result}=    Run Process    os-customization-set    status
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    "active_version":null
    Should Contain    ${result.stdout}    "candidate_state":null

    ${good_artifact}=    Set Variable    ${STATE_DIR}/customization-good.mender
    Create Customization Artifact    ${good_artifact}    site-good-1.0.0    true
    ${result}=    Run Process    mender-update    install    ${good_artifact}
    Should Be Equal As Integers    ${result.rc}    0

    ${result}=    Run Process    os-customization-set    status
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    "candidate_version":"site-good-1.0.0"
    Should Contain    ${result.stdout}    "candidate_state":"pending"

    ${result}=    Run Process    reboot
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Core OS Customization health checks passed.
    ${result}=    Run Process    os-customization-set    status
    Should Contain    ${result.stdout}    "active_version":"site-good-1.0.0"
    Should Contain    ${result.stdout}    "last_good_version":"site-good-1.0.0"
    Should Contain    ${result.stdout}    "candidate_state":null

    ${result}=    Run Process    mender-update    commit
    Should Be Equal As Integers    ${result.rc}    0

    ${failed_artifact}=    Set Variable    ${STATE_DIR}/customization-failed.mender
    Create Customization Artifact    ${failed_artifact}    site-failed-1.0.0    false
    ${result}=    Run Process    mender-update    install    ${failed_artifact}
    Should Be Equal As Integers    ${result.rc}    0
    ${result}=    Run Process    reboot
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Core OS Customization health checks failed.
    ${result}=    Run Process    os-customization-set    status
    Should Contain    ${result.stdout}    "active_version":"site-good-1.0.0"
    Should Contain    ${result.stdout}    "candidate_state":"rolled-back"

    ${result}=    Run Process    mender-update    rollback
    Should Be Equal As Integers    ${result.rc}    0


*** Keywords ***
Setup Environment
    ${version}=    Get Environment Variable    MOCK_MENDER_VERSION    4
    Set Suite Variable    ${MENDER_VERSION}    ${version}
    IF    '${version}' == '5'
        Set Suite Variable    ${COMMIT_ROLLBACK_OUTPUT}    Committing failed.\nRolled back.
        Set Suite Variable    ${INCONSISTENT_OUTPUT}    Update Module does not support rollback. System may be in an inconsistent state.
    ELSE
        Set Suite Variable    ${COMMIT_ROLLBACK_OUTPUT}    Installation failed. Rolled back modifications.
        Set Suite Variable    ${INCONSISTENT_OUTPUT}    Installation failed, and Update Module does not support rollback. System may be in an inconsistent state.
    END
    ${path}=    Get Environment Variable    PATH
    ${newpath}=    Set Variable    ${EXECDIR}/bin:${path}
    Set Environment Variable    PATH    ${newpath}
    Remove Directory    ${STATE_DIR}    recursive=True
    Set Environment Variable    MOCK_MENDER_STATE_DIR     ${STATE_DIR}
    Run Process  preparefs

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

Create Customization Artifact
    [Arguments]    ${artifact}    ${version}    ${health_command}
    ${work_dir}=    Set Variable    ${STATE_DIR}/customization-artifact-${version}
    ${header_dir}=    Set Variable    ${work_dir}/header
    ${payload_dir}=    Set Variable    ${work_dir}/payload
    Create Directory    ${header_dir}
    Create Directory    ${payload_dir}
    Create Directory    ${work_dir}/data
    Create File    ${header_dir}/header-info    {"payloads":[{"type":"os-customization"}],"artifact_depends":{"device_type":["moducop-cpu01"]}}
    Create File    ${payload_dir}/manifest.json    {"format_version":1,"version":"${version}","health_checks":[{"type":"command","command":["${health_command}"]}]}
    ${result}=    Run Process    tar    -C    ${header_dir}    -cf    ${work_dir}/header.tar    header-info
    Should Be Equal As Integers    ${result.rc}    0
    ${result}=    Run Process    tar    -C    ${payload_dir}    -cf    ${work_dir}/data/0000.tar    manifest.json
    Should Be Equal As Integers    ${result.rc}    0
    ${result}=    Run Process    tar    -C    ${work_dir}    -cf    ${artifact}    header.tar    data/0000.tar
    Should Be Equal As Integers    ${result.rc}    0
