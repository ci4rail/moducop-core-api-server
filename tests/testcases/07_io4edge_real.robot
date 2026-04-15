# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

*** Settings ***
Resource     common.resource
Test Tags    io4edge-real

*** Variables ***
${DEV1}  S101-CPU01UC
${FW_NAME1}  fw-cpu01uc-default
# ${DEV2}  S101-IOU06-USB-EXT1

*** Test Cases ***

List Io4Edge Devices Shall Return All Devices
    Sleep  60s  # Wait for SUT to be fully up (server restart has been running before this test)
    
    ${response}=    GET   ${API_URL}/hardware/io4edge-devices  expected_status=any
    Should Be Equal As Integers    ${response.status_code}    200
    Log To Console   ${response.json()} ${response.status_code}

    @{expected_devices}=    Create List
    ...    ${DEV1}

    FOR    ${dev}    IN    @{expected_devices}
        Should Contain    ${response.json()}    ${dev}
    END    


Original Firmware Shall be Reported
    Check Current Version   ${API_URL}/software/io4edge/${DEV1}   
    ...    ${FW_NAME1}
    ...    1.1.0

Initial Firmware Update Shall Pass
    ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV1}  
    ...    ${ASSET_DIR}/fw-cpu01uc-default-1.1.1-rc.1.fwpkg
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    ${status_response}=    Wait for Update    ${API_URL}/software/io4edge/${DEV1}  timeout=60s

    Check Current Version And Deploy Status  ${API_URL}/software/io4edge/${DEV1}   
    ...    ${FW_NAME1}
    ...    1.1.1-rc.1
    ...    success

Firmware Already Deployed Update Shall be NOP
    ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV1}  
    ...    ${ASSET_DIR}/fw-cpu01uc-default-1.1.1-rc.1.fwpkg
    Check Error Status from Response  ${response}   409   io4e-0002
    

Update Shall be Rejected if already an Update in Progress
    ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV1}  
    ...    ${ASSET_DIR}/fw-cpu01uc-default-1.1.0.fwpkg
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    Sleep  2s
    ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV1}  
    ...    ${ASSET_DIR}/fw-cpu01uc-default-1.1.1-rc.1.fwpkg
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    412

    ${status_response}=    Wait for Update    ${API_URL}/software/io4edge/${DEV1}  timeout=60s
    Check Current Version And Deploy Status  ${API_URL}/software/io4edge/${DEV1}       
    ...    ${FW_NAME1}
    ...    1.1.0
    ...    success

Damaged Firmware Update Shall be Rejected
    ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV1} 
    ...    ${ASSET_DIR}/fw-iou16-00-default-damaged.fwpkg
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    400


# Parallel Updates on different Devices Shall Pass
#     ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV_BASE_NAME}2  
#     ...    ${ASSET_DIR}/fw-iou16-00-default-1.0.2.fwpkg
#     Log To Console    ${response.text} ${response.status_code} 
#     Should Be Equal As Integers    ${response.status_code}    202

#     ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV_BASE_NAME}3  
#     ...    ${ASSET_DIR}/fw-iou16-00-default-1.0.3.fwpkg
#     Log To Console    ${response.text} ${response.status_code} 
#     Should Be Equal As Integers    ${response.status_code}    202    

#     ${status_response}=    Wait for Update    ${API_URL}/software/io4edge/${DEV_BASE_NAME}2  timeout=20s
#     Check Current Version And Deploy Status  ${API_URL}/software/io4edge/${DEV_BASE_NAME}2  
#     ...    ${FW_NAME}
#     ...    1.0.2
#     ...    success

#     ${status_response}=    Wait for Update    ${API_URL}/software/io4edge/${DEV_BASE_NAME}3  timeout=5s
#     Check Current Version And Deploy Status  ${API_URL}/software/io4edge/${DEV_BASE_NAME}3  
#     ...    ${FW_NAME}    
#     ...    1.0.3
#     ...    success

# Server Restart While Update Shall Shall Result in Update Resume
# This is no longer supprted!
#     ${response}=    Load Artifact  ${API_URL}/software/io4edge/${DEV1}  
#     ...    ${ASSET_DIR}/fw-cpu01uc-default-1.1.1-rc.1.fwpkg
#     Log To Console    ${response.text} ${response.status_code} 
#     Should Be Equal As Integers    ${response.status_code}    202

#     Crash and Restart SUT

#     ${status_response}=    Wait for Update    ${API_URL}/software/io4edge/${DEV1}  timeout=20s
#     Check Current Version And Deploy Status  ${API_URL}/software/io4edge/${DEV1}  
#     ...    ${FW_NAME1}
#     ...    1.1.1-rc.1
#     ...    success
