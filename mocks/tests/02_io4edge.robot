# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

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
    Should Contain    ${result.stdout}    Firmware name: fw-iou16-00-default, Version 1.0.0

Firmware Update Shall be Installed
    ${result}=    Run Process   io4edge-cli   -d   S100-IUO16-USB-EXT1-UC1  load-firmware   ${EXECDIR}/../tests/assets/fw-iou16-00-default-1.0.2.fwpkg
    Log To Console    ${result.stdout}
    Log To Console    ${result.stderr}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Firmware name: fw-iou16-00-default, Version 1.0.2
    ${result}=    Run Process   io4edge-cli   -d   S100-IUO16-USB-EXT1-UC1   fw
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Firmware name: fw-iou16-00-default, Version 1.0.2
    ${result}=    Run Process   io4edge-cli   -d   S100-IUO16-USB-EXT1-UC2   fw
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Firmware name: fw-iou16-00-default, Version 1.0.0

Device Shall Be Temporarily Unavailable During Post-Update Restart
    ${handle}=    Start Process
    ...    io4edge-cli
    ...    -d
    ...    S100-IUO16-USB-EXT1-UC3
    ...    load-firmware
    ...    ${EXECDIR}/../tests/assets/fw-iou16-00-default-1.0.3.fwpkg
    ...    stdout=PIPE
    ...    stderr=PIPE
    Sleep    10s
    Wait Until Keyword Succeeds    5s    1s    Device Firmware Request Shall Fail As Temporarily Unavailable    S100-IUO16-USB-EXT1-UC3
    Scan Shall Not List Device    S100-IUO16-USB-EXT1-UC3

    ${update}=    Wait For Process    ${handle}
    Should Be Equal As Integers    ${update.rc}    0
    Should Contain    ${update.stdout}    Reconnecting to restarted device.
    Should Contain    ${update.stdout}    Firmware name: fw-iou16-00-default, Version 1.0.3

    ${result}=    Run Process   io4edge-cli   -d   S100-IUO16-USB-EXT1-UC3   fw
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Firmware name: fw-iou16-00-default, Version 1.0.3

    ${result}=    Run Process   io4edge-cli   scan
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    S100-IUO16-USB-EXT1-UC3 192.168.202.1

*** Keywords ***
Setup Environment
    ${path}=    Get Environment Variable    PATH
    ${newpath}=    Set Variable    ${EXECDIR}/bin:${path}
    Set Environment Variable    PATH    ${newpath}
    Remove Directory    ${STATE_DIR}    recursive=True
    Set Environment Variable    MOCK_MENDER_STATE_DIR     ${STATE_DIR}
    Run Process    make    io4edge-cli

Device Firmware Request Shall Fail As Temporarily Unavailable
    [Arguments]    ${device}
    ${result}=    Run Process   io4edge-cli   -d   ${device}   fw
    Should Not Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stderr}    temporarily unavailable

Scan Shall Not List Device
    [Arguments]    ${device}
    ${result}=    Run Process   io4edge-cli   scan
    Should Be Equal As Integers    ${result.rc}    0
    Should Not Contain    ${result.stdout}    ${device}
