# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

*** Settings ***
Resource    common.resource
Test Tags    mender    core-os-customization

*** Test Cases ***
Core OS Customization Factory Version Shall Be Reported
    ${response}=    GET    ${API_URL}/software/core-os-customization/factory    expected_status=any
    Should Be Equal As Integers    ${response.status_code}    200
    Should Be Equal    ${response.json()['factory_version']}    ${None}

Core OS Customization Update Shall Pass
    ${artifact}=    Set Variable    ${STATE_DIR}/site-good.mender
    Create Customization Artifact    ${artifact}    site-good-1.0.0    true

    ${response}=    Load Artifact    ${API_URL}/software/core-os-customization    ${artifact}
    Should Be Equal As Integers    ${response.status_code}    202

    ${status_response}=    Wait for Update    ${API_URL}/software/core-os-customization    timeout=60s
    Check Current Version from Response    ${status_response}    core-os-customization    site-good-1.0.0
    Check Deploy Status from Response    ${status_response}    success    Update deployed successfully

Core OS Customization Already Deployed Shall Be Rejected
    ${artifact}=    Set Variable    ${STATE_DIR}/site-good.mender
    ${response}=    Load Artifact    ${API_URL}/software/core-os-customization    ${artifact}
    Check Error Status from Response    ${response}    409    cpm-0005

Core OS Customization Failed Health Check Shall Fail Deployment
    ${artifact}=    Set Variable    ${STATE_DIR}/site-failed.mender
    Create Customization Artifact    ${artifact}    site-failed-1.0.0    false

    ${response}=    Load Artifact    ${API_URL}/software/core-os-customization    ${artifact}
    Should Be Equal As Integers    ${response.status_code}    202
    ${status_response}=    Wait for Update    ${API_URL}/software/core-os-customization    timeout=60s
    Check Deploy Status from Response    ${status_response}    failure    Core OS customization health checks failed
    Check Current Version from Response    ${status_response}    core-os-customization    site-good-1.0.0

*** Keywords ***
Create Customization Artifact
    [Arguments]    ${artifact}    ${version}    ${health_command}
    ${work_dir}=    Set Variable    ${STATE_DIR}/customization-artifact-${version}
    ${header_dir}=    Set Variable    ${work_dir}/header
    ${payload_dir}=    Set Variable    ${work_dir}/payload
    Create Directory    ${header_dir}
    Create Directory    ${payload_dir}
    Create Directory    ${work_dir}/data
    Create File    ${header_dir}/header-info    {"payloads":[{"type":"os-customization"}],"artifact_depends":{"device_type":["moducop-cpu01"]}}
    Create File    ${payload_dir}/manifest.json    {"format_version":1,"version":"${version}","health_checks":[{"type":"command","command":["${health_command}"]}]}
    ${result}=    Run Process    tar    -C    ${header_dir}    -cf    ${work_dir}/header.tar    header-info
    Should Be Equal As Integers    ${result.rc}    0
    ${result}=    Run Process    tar    -C    ${payload_dir}    -cf    ${work_dir}/data/0000.tar    manifest.json
    Should Be Equal As Integers    ${result.rc}    0
    ${result}=    Run Process    tar    -C    ${work_dir}    -cf    ${artifact}    header.tar    data/0000.tar
    Should Be Equal As Integers    ${result.rc}    0
