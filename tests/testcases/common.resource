# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0


*** Settings ***
Library   OperatingSystem
Library      RequestsLibrary
Library  Process
Library  SSHLibrary
Resource  ../robot-helpers/sshlib/utils.resource
Variables  ../duts.yaml

*** Variables ***
${DUT_IP}       \
${API_URL}        http://localhost:8090/api/v1
${ASSET_DIR}      ${EXECDIR}/assets
${STATE_DIR}      ${EXECDIR}/../mock-mender-state
${MOCK_FS_PATH}   ${STATE_DIR}/fs
${SUT_LOGFILE}    ${EXECDIR}/results/sut.log
${SIMULATION_MODE}  true
${COREOS_IMAGE1}  ${ASSET_DIR}/Moducop-CPU01_Standard-Image_v2.7.0.40ee657.20260218.1208.mender
${COREOS_IMAGE1-VERSION}  v2.7.0.40ee657.20260218.1208
${COREOS_IMAGE2}  ${ASSET_DIR}/Moducop-CPU01_Standard-Image_dirty_v2.7.0.some_dummy_change.40ee657.klaus.20260313.1713.mender
${COREOS_IMAGE2-VERSION}  v2.7.0.some_dummy_change.40ee657.klaus.20260313.1713

*** Keywords ***

Start SUT
    IF  '${SIMULATION_MODE}' == 'true'   
        Start Process  ./run_sim_endless.sh  shell=True  stdout=${SUT_LOGFILE}  stderr=${SUT_LOGFILE}
    END
    
Crash and Restart SUT
    IF  '${SIMULATION_MODE}' == 'true'  
        Sleep  1.5s
        Terminate All Processes
        Start SUT
        Sleep  3s
    ELSE
        Set To Dictionary    ${duts.DUT1}  IP_ADDRESS=${DUT_IP}
        Open SSH Connection to DUT
        Sleep  1.5s
        Log To Console    Rebooting DUT
        SSH Command Should Succeed    reboot
        Sleep  60s
        Open SSH Connection to DUT
    END

Load Artifact
    [Arguments]    ${URL}   ${artifact_path}
    ${file_data}=    Get Binary File    ${artifact_path}
    ${response}=    PUT   ${URL}   data=${file_data}  expected_status=any
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
    Should Be Equal As Integers    ${response.status_code}    200
    Log To Console   ${response.json()} ${response.status_code}
    Check Current Version from Response   ${response}  ${expected_name}   ${expected_version}

Check Current Version from Response
    [Arguments]    ${response}  ${expected_name}   ${expected_version}
    Should Be Equal As Integers    ${response.status_code}    200
    Log To Console   ${response.json()} ${response.status_code}
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

Check Current Version And Deploy Status
    [Arguments]    ${URL}  ${expected_name}   ${expected_version}  ${expected_code}  ${expected_message}=${NONE}
    ${response}=    GET   ${URL}  expected_status=any
    Should Be Equal As Integers    ${response.status_code}    200
    Log To Console   ${response.json()} ${response.status_code}
    Should Be Equal as Strings    ${response.json()['current']['name']}    ${expected_name}
    Should Be Equal as Strings    ${response.json()['current']['version']}    ${expected_version}
    Check Deploy Status from Response   ${response}  ${expected_code}  ${expected_message}

Clear Error Injection
    ${result}=    Run Process   mender-update   err-inject   \
    Should Be Equal As Integers    ${result.rc}    0
