# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

*** Settings ***
Resource     common.resource
Test Tags    hardware

*** Test Cases ***

Get Hardware Info Shall Pass
    ${response}=    GET  ${API_URL}/hardware
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    200
    IF  '${SIMULATION_MODE}' == 'true' 
        Should Be Equal as Strings  ${response.json()['vendor']}    Ci4Rail
        Should Be Equal as Strings  ${response.json()['model']}     S100-MLC01
        Should Be Equal as Strings  ${response.json()['serial']}    12345
        Should Be Equal as Numbers  ${response.json()['majorVersion']}    1
        Should Be Equal as Numbers  ${response.json()['variant']}    0
    ELSE
        Should Be Equal as Strings  ${response.json()['vendor']}    Ci4Rail
        Should Be Equal as Strings  ${response.json()['model']}     S100-MEC01
        Should Match Regexp         ${response.json()['serial']}    ^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-4[0-9A-Fa-f]{3}-[89ABab][0-9A-Fa-f]{3}-[0-9A-Fa-f]{12}$
        Should Be Equal as Numbers  ${response.json()['majorVersion']}    1
        Should Be Equal as Numbers  ${response.json()['variant']}    0
    END
