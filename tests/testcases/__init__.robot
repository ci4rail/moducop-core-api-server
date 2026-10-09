# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

*** Settings ***
Library  OperatingSystem
Library  Process
Library  Collections
Resource  common.resource
Resource  ../robot-helpers/sshlib/utils.resource

Suite Setup  Prepare Environment
Suite Teardown  Clear Environment



*** Variables ***


*** Keywords ***

Prepare Environment
    IF    "${DUT_IP}" != ""
        Log To Console    Running in non-simulation mode, using DUT IP ${DUT_IP}
        Set Global Variable  ${API_URL}  http://${DUT_IP}:8090/api/v1
        Set Global Variable  ${SIMULATION_MODE}  false

        Set To Dictionary    ${duts.DUT1}  IP_ADDRESS=${DUT_IP}
        Log To Console    DUTS: ${DUTS}
        Open SSH Connection to DUT

    ELSE    
        Log To Console    Running in simulation mode
        # The mock uses legacy rootfs-image fixtures; the target images use
        # bootfit-rootfs, which is not yet modeled by the mock.
        Set Global Variable    ${COREOS_IMAGE1}    ${ASSET_DIR}/Moducop-CPU01_Standard-Image_v2.7.0.40ee657.20260218.1208.mender
        Set Global Variable    ${COREOS_IMAGE1-VERSION}    v2.7.0.40ee657.20260218.1208
        Set Global Variable    ${COREOS_IMAGE2}    ${ASSET_DIR}/Moducop-CPU01_Standard-Image_dirty_v2.7.0.some_dummy_change.40ee657.klaus.20260313.1713.mender
        Set Global Variable    ${COREOS_IMAGE2-VERSION}    v2.7.0.some_dummy_change.40ee657.klaus.20260313.1713

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
    IF  '${SIMULATION_MODE}' == 'true'
        Terminate All Processes
    END
