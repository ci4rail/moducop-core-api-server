# EXECDIR must be mocks/

*** Settings ***
Library    Process
Library    OperatingSystem
Suite Setup    Setup Environment
Test Tags  io4edge

*** Variables ***
${STATE_DIR}    ${EXECDIR}/tests/mock-mender-state

*** Test Cases ***

Hardware Identification Shall be correct
    ${result}=    Run Process   io4edge-cli   -d   S100-IUO16-USB-EXT1-UC1   hw
    Should Be Equal As Integers    ${result.rc}    0
    Log To Console    ${result.stdout}
    Should Contain    ${result.stdout}    Hardware name: S100-IUO16-00-00001, rev: 0, serial: b4e31793-f660-4e2e-af20-c175186b95be

Scan Shall list all known devices
    ${result}=    Run Process   io4edge-cli   scan
    Should Be Equal As Integers    ${result.rc}    0
    Log To Console    ${result.stdout}
    Should Contain    ${result.stdout}    DEVICE ID
    Should Contain    ${result.stdout}    S100-IUO16-USB-EXT1-UC1 192.168.200.1
    Should Contain    ${result.stdout}    S100-IUO16-USB-EXT1-UC2 192.168.201.1
    Should Contain    ${result.stdout}    S100-IUO16-USB-EXT1-UC3 192.168.202.1
    Should Contain    ${result.stdout}    S100-IUO16-USB-EXT1-UC4 192.168.203.1
    Should Contain    ${result.stdout}    S100-IUO16-USB-EXT1-UC5 192.168.204.1

Initial Firmware Identification shall be 1.0.0
    ${result}=    Run Process   io4edge-cli   -d   S100-IUO16-USB-EXT1-UC1   fw
    Should Be Equal As Integers    ${result.rc}    0
    Log To Console    ${result.stdout}
    Should Contain    ${result.stdout}    Firmware name: fw_iou016_default, Version 1.0.0

Firmware Update Shall be Installed
    ${result}=    Run Process   io4edge-cli   -d   S100-IUO16-USB-EXT1-UC1  load-firmware   ${EXECDIR}/../tests/assets/fw-iou16-00-default-1.0.2.fwpkg
    Log To Console    ${result.stdout}
    Log To Console    ${result.stderr}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Firmware name: fw_iou016_default, Version 1.0.2
    ${result}=    Run Process   io4edge-cli   -d   S100-IUO16-USB-EXT1-UC1   fw
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Firmware name: fw_iou016_default, Version 1.0.2
    ${result}=    Run Process   io4edge-cli   -d   S100-IUO16-USB-EXT1-UC2   fw
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Firmware name: fw_iou016_default, Version 1.0.0

*** Keywords ***
Setup Environment
    ${path}=    Get Environment Variable    PATH
    ${newpath}=    Set Variable    ${EXECDIR}/bin:${path}
    Set Environment Variable    PATH    ${newpath}
    Remove Directory    ${STATE_DIR}    recursive=True
    Set Environment Variable    MOCK_MENDER_STATE_DIR     ${STATE_DIR}
    Run Process    make    io4edge-cli
    