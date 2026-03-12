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

    # Kill && Restart Server
    Sleep  1.5s
    Terminate All Processes
    Start SUT
    Sleep  3s

    ${status_response}=    Wait for Update    ${API_URL}/software/application/${APP_NAME}  timeout=10s
    Check Current Version  ${API_URL}/software/application/${APP_NAME} 
    ...   nginx-demo
    ...   8f249b9

