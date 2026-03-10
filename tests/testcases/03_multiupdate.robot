*** Settings ***
Resource     common.resource

*** Variables ***
${APP_NAME}  nginx-demo 

*** Test Cases ***
Simultaneous CoreOS and App Update Shall Pass
    ${response}=    Load Artifact  ${API_URL}/software/core-os  
    ...    ${ASSET_DIR}/Moducop-CPU01_Standard-Image_v2.6.0.f457f6d.20260210.1540.mender
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    ${response}=    Load Artifact  ${API_URL}/software/application/${APP_NAME}  
    ...    ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-a895c3c.mender
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    Wait for Update    ${API_URL}/software/application/${APP_NAME}  timeout=60s
    Check Current Version  ${API_URL}/software/application/${APP_NAME} 
    ...   nginx-demo
    ...   a895c3c

    Check Current Version   ${API_URL}/software/core-os   
    ...    cpu01-standard
    ...    v2.6.0.f457f6d.20260210.1540


