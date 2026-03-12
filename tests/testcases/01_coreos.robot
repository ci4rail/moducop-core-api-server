*** Settings ***
Resource     common.resource

*** Variables ***
${APP_NAME}  nginx-demo 

*** Test Cases ***
Original Rootfs Shall be Reported
    Check Current Version   ${API_URL}/software/core-os   
    ...    cpu01-standard
    ...    v2.6.0.f457f6d.20260210.1540

Initial CoreOS Update Shall Pass
    ${response}=    Load Artifact  ${API_URL}/software/core-os  
    ...    ${ASSET_DIR}/Moducop-CPU01_Standard-Image_v2.7.0.40ee657.20260218.1208.mender
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    ${status_response}=    Wait for Update    ${API_URL}/software/core-os  timeout=60s

    Check Current Version   ${API_URL}/software/core-os   
    ...    cpu01-standard
    ...    v2.7.0.40ee657.20260218.1208

    Check Deploy Status from Response   ${status_response}   success   Update deployed successfully

CoreOS Already Deployed Update Shall be NOP
    ${response}=    Load Artifact  ${API_URL}/software/core-os  
    ...    ${ASSET_DIR}/Moducop-CPU01_Standard-Image_v2.7.0.40ee657.20260218.1208.mender
    Check Error Status from Response  ${response}   409   cpm-0005
