*** Settings ***
Library  OperatingSystem
Library  Process
Resource  common.resource

Suite Setup  Prepare Environment
Suite Teardown  Clear Environment

*** Variables ***


*** Keywords ***

Prepare Environment
    IF    "${DUT_IP}" != ""
        Log To Console    Running in non-simulation mode, using DUT IP ${DUT_IP}
        Set Global Variable  ${API_URL}  http://${DUT_IP}:8090/api/v1
        Set Global Variable  ${SIMULATION_MODE}  false
    ELSE    
        Log To Console    Running in simulation mode
        ${path}=    Get Environment Variable    PATH
        ${newpath}=    Set Variable    ${EXECDIR}/../mocks/bin:${path}
        Set Environment Variable    PATH    ${newpath}
        Remove Directory    ${STATE_DIR}    recursive=True
        Set Environment Variable    MOCK_MENDER_STATE_DIR     ${STATE_DIR}
        Set Environment Variable    MOCK_MENDER_KILL_PARENT    yes
        Run Process  preparefs
        Start SUT
        Sleep  3s
    END

Clear Environment
    IF  '$SIMULATION_MODE == true'
        Terminate All Processes
    END
