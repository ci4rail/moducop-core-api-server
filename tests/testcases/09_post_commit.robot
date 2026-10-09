# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
# SPDX-License-Identifier: Apache-2.0

*** Settings ***
Resource    common.resource
Test Tags    mender    application
Test Template    Committed Update Shall Survive Subsequent Failure

*** Test Cases ***
Post Commit Failure Shall Preserve Installed Application    post-commit-failed    a895c3c
Cleanup Failure Shall Preserve Installed Application    cleanup-failed    8f249b9

*** Keywords ***
Committed Update Shall Survive Subsequent Failure
    [Arguments]    ${point}    ${version}
    IF    '${SIMULATION_MODE}' != 'true'
        SKIP    Requires mock error injection
    END
    # Each failure scenario must deploy a version different from the active one.
    Clear Error Injection
    ${baseline}=    Set Variable If    '${version}' == 'a895c3c'    8f249b9    a895c3c
    ${current}=    GET    ${API_URL}/software/application/nginx-demo    expected_status=any
    IF    ${current.status_code} != 200 or $current.json()['current']['version'] != $baseline
        ${response}=    Load Artifact    ${API_URL}/software/application/nginx-demo    ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-${baseline}.mender
        Should Be Equal As Integers    ${response.status_code}    202
        ${status}=    Wait for Update    ${API_URL}/software/application/nginx-demo
        Check Deploy Status from Response    ${status}    success
    END
    Check Current Version    ${API_URL}/software/application/nginx-demo    nginx-demo    ${baseline}

    ${result}=    Run Process    mender-update    err-inject    ${point}
    Should Be Equal As Integers    ${result.rc}    0
    TRY
        ${response}=    Load Artifact    ${API_URL}/software/application/nginx-demo    ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-${version}.mender
        Should Be Equal As Integers    ${response.status_code}    202
        ${status}=    Wait for Update    ${API_URL}/software/application/nginx-demo
        Check Deploy Status from Response    ${status}    failure
        Should Contain    ${status.json()['deploy_status']['message']}    committed the update
        Check Current Version    ${API_URL}/software/application/nginx-demo    nginx-demo    ${version}
        ${result}=    Run Process    docker    ps    --format    {{.Names}}\\t{{.Labels}}
        Should Contain    ${result.stdout}    nginx-demo-web-1
    FINALLY
        Clear Error Injection
    END
