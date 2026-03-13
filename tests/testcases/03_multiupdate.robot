*** Settings ***
Resource     common.resource
Test Tags  mender  coreos  application

*** Variables ***
${APP_NAME}  nginx-demo 

*** Test Cases ***
Simultaneous CoreOS and App Update Shall Pass
    ${response}=    Load Artifact  ${API_URL}/software/core-os  ${COREOS_IMAGE1}
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
    ...    ${COREOS_IMAGE1-VERSION}


