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

    Clear Error Injection

    ${status_response}=    Wait for Update    ${API_URL}/software/application/${APP_NAME}  timeout=120s
    Check Current Version  ${API_URL}/software/application/${APP_NAME} 
    ...   nginx-demo
    ...   a895c3c
    Check Deploy Status from Response   ${status_response}   success   Update deployed successfully

Persistent docker compose failure shall stop after bounded recovery retries

    ${result}=    Run Process   mender-update   err-inject   docker-compose-up-failed
    Should Be Equal As Integers    ${result.rc}    0

    ${response}=    Load Artifact  ${API_URL}/software/application/${APP_NAME}  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
    Should Be Equal As Integers    ${response.status_code}    202

    ${status_response}=    Wait for Update    ${API_URL}/software/application/${APP_NAME}  timeout=60s
    Check Deploy Status from Response   ${status_response}   failure

    ${result}=  Run Docker PS WithLabels
    Should Not Contain  ${result.stdout}    nginx-demo-web-1

    Clear Error Injection

*** Keywords ***
Run Docker PS WithLabels
    ${result}=    Run Process    docker  ps   --format  {{.Names}}\\t{{.Labels}}
    Log To Console    ${result.stderr}
    Log To Console    ${result.stdout}

    RETURN    ${result}
