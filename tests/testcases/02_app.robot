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