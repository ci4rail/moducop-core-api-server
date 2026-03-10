*** Settings ***
Library  OperatingSystem
Library  Process
Resource  common.resource

Suite Setup  Prepare Environment
Suite Teardown  Clear Environment

*** Variables ***


*** Keywords ***

Prepare Environment
    ${path}=    Get Environment Variable    PATH
    ${newpath}=    Set Variable    ${EXECDIR}/../mocks/bin:${path}
    Set Environment Variable    PATH    ${newpath}
    Remove Directory    ${STATE_DIR}    recursive=True
    Set Environment Variable    MOCK_MENDER_STATE_DIR     ${STATE_DIR}
    Set Environment Variable    MOCK_MENDER_KILL_PARENT    yes
    Run Process  preparefs
    Start Process  ./run_sim_endless.sh  shell=True  stdout=${SUT_LOGFILE}  stderr=${SUT_LOGFILE}
    Sleep  3s

Clear Environment
    Terminate All Processes
