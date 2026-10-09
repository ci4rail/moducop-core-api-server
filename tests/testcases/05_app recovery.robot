# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

*** Settings ***
Resource     common.resource

Test Tags  mender  application

*** Variables ***
${APP_NAME}  nginx-demo 

*** Test Cases ***
Reboot during application update shall recover and complete update

    IF  '${SIMULATION_MODE}' != 'true' 
        SKIP   same as 04_serverrestart.robot for non-simulation mode
    END
    ${result}=    Run Process   mender-update   err-inject  after-stop-old-containers
    Log To Console    ${result.stdout} ${result.stderr} ${result.rc}
    Should Be Equal As Integers    ${result.rc}    0

    ${response}=    Load Artifact  ${API_URL}/software/application/${APP_NAME}  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-a895c3c.mender
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    Wait Until Keyword Succeeds    10s    100ms    Application Interruption Checkpoint Is Saved
    Wait Until Keyword Succeeds    10s    100ms    Clear Error Injection

    ${status_response}=    Wait for Update    ${API_URL}/software/application/${APP_NAME}  timeout=120s
    Check Current Version  ${API_URL}/software/application/${APP_NAME} 
    ...   nginx-demo
    ...   a895c3c
    Check Deploy Status from Response   ${status_response}   success   Update deployed successfully

Persistent docker compose failure shall fail and restore the previous application
    [Teardown]    Run Keyword If    '${SIMULATION_MODE}' == 'true'    Clear Error Injection

    IF  '${SIMULATION_MODE}' != 'true'
        SKIP    requires mock error injection
    END

    # Establish the known-good version independently of the preceding test.
    Clear Error Injection
    ${current}=    GET    ${API_URL}/software/application/${APP_NAME}    expected_status=any
    IF    ${current.status_code} != 200 or $current.json()['current']['version'] != 'a895c3c'
        ${response}=    Load Artifact  ${API_URL}/software/application/${APP_NAME}  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-a895c3c.mender
        Should Be Equal As Integers    ${response.status_code}    202
        ${status_response}=    Wait for Update    ${API_URL}/software/application/${APP_NAME}
        Check Deploy Status from Response    ${status_response}    success
    END
    Check Current Version    ${API_URL}/software/application/${APP_NAME}    nginx-demo    a895c3c

    ${result}=    Run Process   mender-update   err-inject   docker-compose-up-failed
    Should Be Equal As Integers    ${result.rc}    0

    ${response}=    Load Artifact  ${API_URL}/software/application/${APP_NAME}  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
    Should Be Equal As Integers    ${response.status_code}    202

    ${status_response}=    Wait for Update    ${API_URL}/software/application/${APP_NAME}  timeout=60s
    Check Deploy Status from Response   ${status_response}   failure

    ${result}=  Run Docker PS WithLabels
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    nginx-demo-web-1
    Should Contain    ${result.stdout}    nginx-demo-tester-1
    Should Contain    ${result.stdout}    com.ci4rail.app.software-version=a895c3c
    Should Not Contain    ${result.stdout}    com.ci4rail.app.software-version=8f249b9
    Check Current Version from Response    ${status_response}    nginx-demo    a895c3c
    Should Contain    ${status_response.json()['deploy_status']['message']}    Rolled back

    # A terminal rollback must remain failed while the injection is still active.
    Sleep    3s
    Check Current Version And Deploy Status    ${API_URL}/software/application/${APP_NAME}
    ...    nginx-demo    a895c3c    failure

*** Keywords ***
Application Interruption Checkpoint Is Saved
    ${data}=    OperatingSystem.Get File    ${STATE_DIR}/state.json
    ${state}=    Evaluate    json.loads($data)    modules=json
    Should Be Equal    ${state['install_phase']}    app-stopped


Run Docker PS WithLabels
    ${result}=    Run Process    docker  ps   --format  {{.Names}}\\t{{.Labels}}
    Log To Console    ${result.stderr}
    Log To Console    ${result.stdout}

    RETURN    ${result}
