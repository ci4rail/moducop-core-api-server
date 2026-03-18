# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

*** Settings ***
Resource     common.resource
Test Tags  mender  coreos

*** Variables ***
${APP_NAME}  nginx-demo 

*** Test Cases ***
Original Rootfs Shall be Reported
    Check Current Version   ${API_URL}/software/core-os   
    ...    cpu01-standard
    ...    ${COREOS_IMAGE1-VERSION}

Initial CoreOS Update Shall Pass
    ${response}=    Load Artifact  ${API_URL}/software/core-os  ${COREOS_IMAGE2} 
    Log To Console    ${response.text} ${response.status_code}     
    Should Be Equal As Integers    ${response.status_code}    202

    ${status_response}=    Wait for Update    ${API_URL}/software/core-os  timeout=600s

    Check Current Version   ${API_URL}/software/core-os   
    ...    cpu01-standard
    ...    ${COREOS_IMAGE2-VERSION}

    Check Deploy Status from Response   ${status_response}   success   Update deployed successfully

CoreOS Already Deployed Update Shall be NOP
    ${response}=    Load Artifact  ${API_URL}/software/core-os  
    ...    ${COREOS_IMAGE2}
    Check Error Status from Response  ${response}   409   cpm-0005
