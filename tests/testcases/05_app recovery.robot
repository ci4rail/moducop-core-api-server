*** Settings ***
Resource     common.resource
Test Tags  mender  application

*** Variables ***
${APP_NAME}  nginx-demo 

*** Test Cases ***
Reboot during application update shall recover and complete update

    IF  '${SIMULATION_MODE}' != 'true' 
        SKIP   same as 04_serverrestart.robot for non-simulation mode
    END
    ${result}=    Run Process   mender-update   err-inject  after-stop-old-containers
    Log To Console    ${result.stdout} ${result.stderr} ${result.rc}
    Should Be Equal As Integers    ${result.rc}    0

    ${response}=    Load Artifact  ${API_URL}/software/application/${APP_NAME}  ${ASSET_DIR}/app-nginx-demo-moducop-cpu01-linux_arm64-a895c3c.mender
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    202

    Clear Error Injection

    ${status_response}=    Wait for Update    ${API_URL}/software/application/${APP_NAME}  timeout=120s
    Check Current Version  ${API_URL}/software/application/${APP_NAME} 
    ...   nginx-demo
    ...   a895c3c
    Check Deploy Status from Response   ${status_response}   success   Update deployed successfully

