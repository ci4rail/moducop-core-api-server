*** Settings ***
Library   OperatingSystem
Library      RequestsLibrary

*** Variables ***
${API_URL}        http://localhost:8090/api/v1
${ASSET_DIR}      ${EXECDIR}/assets
${STATE_DIR}      ${EXECDIR}/../mock-mender-state

*** Keywords ***

Load Artifact
    [Arguments]    ${URL}   ${artifact_path}
    ${file_data}=    Get Binary File    ${artifact_path}
    ${response}=    PUT   ${URL}   data=${file_data}
    RETURN    ${response}

Wait for Update
    [Arguments]    ${URL}     ${timeout}=60s
    Wait Until Keyword Succeeds    ${timeout}    3s    Check Update Finished    ${URL}

Check Update Finished
    [Arguments]    ${URL}
    ${response}=    GET   ${URL}
    Log To Console   ${response.json()} ${response.status_code}
    ${code}=   Set Variable    ${response.json()['deploy_status']['code']}
    IF    '${code}' == 'waiting'
        Fail    Update still in progress
    ELSE IF   '${code}' == 'in_progress'
        Fail    Update still in progress
    END