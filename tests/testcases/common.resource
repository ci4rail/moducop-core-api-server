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