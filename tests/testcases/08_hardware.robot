*** Settings ***
Resource     common.resource
Test Tags    hardware

*** Test Cases ***

Get Hardware Info Shall Pass
    ${response}=    GET  ${API_URL}/hardware
    Log To Console    ${response.text} ${response.status_code} 
    Should Be Equal As Integers    ${response.status_code}    200
    Should Be Equal as Strings  ${response.json()['vendor']}    Ci4Rail
    Should Be Equal as Strings  ${response.json()['model']}     S100-MLC01
    Should Be Equal as Strings  ${response.json()['serial']}    12345
    Should Be Equal as Numbers  ${response.json()['majorVersion']}    1
    Should Be Equal as Numbers  ${response.json()['variant']}    0