## [1.1.4]

Initial

## [1.1.5]

### Bug Fixes

* Wrong behavior when no io4edge devices found
* Temporary files not cleaned in all scenarios
* io4edge devices not reliably detected on startup -> no scanning happens on demand. Impact: io4edge updates are not automatically restart after a moducop reboot.


## [2.0.0]

## Breaking Changes

* Upgrade to moducop images 3.x
  * No longer supports mender v4. mender v5 is required.
  * No longer supports old mender application update module. Application update module with rollback support is required.
