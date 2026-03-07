*** Settings ***
Library  OperatingSystem
Library  Process
Resource  common.resource

# Suite Setup  Prepare Environment
# Suite Teardown  Clear Environment

*** Keywords ***

# Prepare Environment
#     Remove Directory    ${STATE_DIR}    recursive=True
