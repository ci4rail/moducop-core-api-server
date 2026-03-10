
*** Settings ***
Library   OperatingSystem
Library      RequestsLibrary
Library  Process

*** Variables ***
${API_URL}        http://localhost:8090/api/v1
${ASSET_DIR}      ${EXECDIR}/assets
${STATE_DIR}      ${EXECDIR}/../mock-mender-state
${SUT_LOGFILE}    ${EXECDIR}/results/sut.log

*** Keywords ***

Start SUT
    Start Process  ./run_sim_endless.sh  shell=True  stdout=${SUT_LOGFILE}  stderr=${SUT_LOGFILE}

Load Artifact
    [Arguments]    ${URL}   ${artifact_path}
    ${file_data}=    Get Binary File    ${artifact_path}
    ${response}=    PUT   ${URL}   data=${file_data}
    RETURN    ${response}

Wait for Update
    [Arguments]    ${URL}     ${timeout}=60s
    ${response}=    Wait Until Keyword Succeeds    ${timeout}    3s    Check Update Finished    ${URL}
    RETURN    ${response}

Check Update Finished
    [Arguments]    ${URL}
    ${response}=    GET   ${URL}  expected_status=any
    Log To Console   ${response.json()} ${response.status_code}
    ${code}=   Set Variable    ${response.json()['deploy_status']['code']}
    IF    '${code}' == 'waiting'
        Fail    Update still in progress
    ELSE IF   '${code}' == 'in_progress'
        Fail    Update still in progress
    END
    RETURN    ${response}
    
Check Current Version
    [Arguments]    ${URL}  ${expected_name}   ${expected_version}
    ${response}=    GET   ${URL}  expected_status=any
    Log To Console   ${response.json()} ${response.status_code}
    Should Be Equal As Integers    ${response.status_code}    200
    Should Be Equal as Strings    ${response.json()['current']['name']}    ${expected_name}
    Should Be Equal as Strings    ${response.json()['current']['version']}    ${expected_version}

Check Error Status from Response
    [Arguments]    ${response}  ${expected_http_status}  ${expected_code}  ${expected_message}=${NONE}
    Log To Console   ${response.json()} ${response.status_code}
    Should Be Equal As Integers    ${response.status_code}    ${expected_http_status}
    Should Be Equal as Strings    ${response.json()['code']}    ${expected_code}
    IF    $expected_message is not None
        Should Be Equal as Strings    ${response.json()['message']}    ${expected_message}
    END

Check Deploy Status from Response
    [Arguments]    ${response}  ${expected_code}  ${expected_message}=${NONE}
    Log to Console  ${response}
    Should Be Equal As Integers    ${response.status_code}    200
    Log To Console   ${response.json()} ${response.status_code}
    Should Be Equal as Strings    ${response.json()['deploy_status']['code']}    ${expected_code}
    IF    $expected_message is not None
        Should Be Equal as Strings    ${response.json()['deploy_status']['message']}    ${expected_message}
    END
