# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

*** Settings ***
Resource     common.resource
Test Tags  mender  application

*** Variables ***
${APP_NAME}  nginx-demo 

*** Test Cases ***
Crash During Update Shall be Recovered
    ${response}=    Load Artifact  ${API_URL}/software/application/${APP_NAME}  
    ...    ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    Crash and Restart SUT

    ${status_response}=    Wait for Update    ${API_URL}/software/application/${APP_NAME}  timeout=180s
    Check Current Version  ${API_URL}/software/application/${APP_NAME} 
    ...   nginx-demo
    ...   8f249b9

