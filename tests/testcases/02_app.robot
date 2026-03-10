*** Settings ***
Resource     common.resource
Library      RequestsLibrary

*** Variables ***
${APP_NAME}  nginx-demo 

*** Test Cases ***
Initial Application Update Shall Pass
    ${response}=    Load Artifact  ${API_URL}/software/application/${APP_NAME}  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    Wait for Update    ${API_URL}/software/application/${APP_NAME}  timeout=20s

    ${response}=    GET   ${API_URL}/software/application/${APP_NAME}
    Log To Console   ${response.json()} ${response.status_code}