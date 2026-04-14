# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

*** Settings ***
Resource     common.resource
Test Tags    io4edge-mock

*** Variables ***
${DEV_BASE_NAME}  S100-IUO16-USB-EXT1-UC
${FW_NAME}  fw-iou16-00-default

*** Test Cases ***

List Io4Edge Devices Shall Return All Devices
    ${response}=    GET   ${API_URL}/hardware/io4edge-devices  expected_status=any
    Should Be Equal As Integers    ${response.status_code}    200
    Log To Console   ${response.json()} ${response.status_code}

    @{expected_devices}=    Create List
    ...    S100-IUO16-USB-EXT1-UC1
    ...    S100-IUO16-USB-EXT1-UC2
    ...    S100-IUO16-USB-EXT1-UC3
    ...    S100-IUO16-USB-EXT1-UC4
    ...    S100-IUO16-USB-EXT1-UC5

    FOR    ${dev}    IN    @{expected_devices}
        Should Contain    ${response.json()}    ${dev}
    END    


Original Firmware Shall be Reported
    Check Current Version   ${API_URL}/software/io4edge/${DEV_BASE_NAME}1   
    ...    ${FW_NAME}
    ...    1.0.0

Initial Firmware Update Shall Pass
    ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV_BASE_NAME}1  
    ...    ${ASSET_DIR}/fw-iou16-00-default-1.0.2.fwpkg
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    ${status_response}=    Wait for Update    ${API_URL}/software/io4edge/${DEV_BASE_NAME}1  timeout=20s

    Check Current Version And Deploy Status  ${API_URL}/software/io4edge/${DEV_BASE_NAME}1   
    ...    ${FW_NAME}
    ...    1.0.2
    ...    success

Firmware Already Deployed Update Shall be NOP
    ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV_BASE_NAME}1  
    ...    ${ASSET_DIR}/fw-iou16-00-default-1.0.2.fwpkg
    Check Error Status from Response  ${response}   409   io4e-0002
    

Update Shall be Rejected if already an Update in Progress
    ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV_BASE_NAME}1  
    ...    ${ASSET_DIR}/fw-iou16-00-default-1.0.3.fwpkg
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    Sleep  2s
    ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV_BASE_NAME}1  
    ...    ${ASSET_DIR}/fw-iou16-00-default-1.0.2.fwpkg
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    412

    ${status_response}=    Wait for Update    ${API_URL}/software/io4edge/${DEV_BASE_NAME}1  timeout=20s
    Check Current Version And Deploy Status  ${API_URL}/software/io4edge/${DEV_BASE_NAME}1   
    ...    ${FW_NAME}
    ...    1.0.3
    ...    success

Damaged Firmware Update Shall be Rejected
    ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV_BASE_NAME}1  
    ...    ${ASSET_DIR}/fw-iou16-00-default-damaged.fwpkg
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    400


Parallel Updates on different Devices Shall Pass
    ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV_BASE_NAME}2  
    ...    ${ASSET_DIR}/fw-iou16-00-default-1.0.2.fwpkg
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV_BASE_NAME}3  
    ...    ${ASSET_DIR}/fw-iou16-00-default-1.0.3.fwpkg
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202    

    ${status_response}=    Wait for Update    ${API_URL}/software/io4edge/${DEV_BASE_NAME}2  timeout=20s
    Check Current Version And Deploy Status  ${API_URL}/software/io4edge/${DEV_BASE_NAME}2  
    ...    ${FW_NAME}
    ...    1.0.2
    ...    success

    ${status_response}=    Wait for Update    ${API_URL}/software/io4edge/${DEV_BASE_NAME}3  timeout=5s
    Check Current Version And Deploy Status  ${API_URL}/software/io4edge/${DEV_BASE_NAME}3  
    ...    ${FW_NAME}    
    ...    1.0.3
    ...    success


