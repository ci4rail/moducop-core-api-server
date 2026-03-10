*** Settings ***
Resource     common.resource
Library      RequestsLibrary

*** Variables ***
${APP_NAME}  nginx-demo 

*** Test Cases ***
Initial CoreOS Update Shall Pass
    ${response}=    Load Artifact  ${API_URL}/software/core-os  ${ASSET_DIR}/Moducop-CPU01_Standard-Image_v2.7.0.40ee657.20260218.1208.mender
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    Wait for Update    ${API_URL}/software/core-os  timeout=60s

    ${response}=    GET   ${API_URL}/software/core-os
    Log To Console   ${response.json()} ${response.status_code}