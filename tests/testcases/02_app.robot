*** Settings ***
Resource     common.resource
Test Tags  mender  application

*** Variables ***
${APP_NAME}  nginx-demo 

*** Test Cases ***
Initial Application Update Shall Pass
    ${response}=    Load Artifact  ${API_URL}/software/application/${APP_NAME}  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    ${status_response}=    Wait for Update    ${API_URL}/software/application/${APP_NAME}  timeout=20s
    Check Current Version  ${API_URL}/software/application/${APP_NAME} 
    ...   nginx-demo
    ...   8f249b9
    Check Deploy Status from Response   ${status_response}   success   Update deployed successfully

Application Already Deployed Update Shall be NOP
    ${response}=    Load Artifact  ${API_URL}/software/application/${APP_NAME}  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender
    Check Error Status from Response  ${response}   409   cpm-0005 


List Applications Shall Return All Applications
    ${response}=    GET   ${API_URL}/software/applications  expected_status=any
    Should Be Equal As Integers    ${response.status_code}    200
    Log To Console   ${response.json()} ${response.status_code}
    @{expected_apps}=    Create List
    ...    nginx-demo

    FOR    ${app}    IN    @{expected_apps}
        Should Contain    ${response.json()}    ${app}
    END